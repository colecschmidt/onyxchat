import { api, setToken } from './client'
import { getOrCreateKeyPair, exportPublicKey, clearKeyPair } from '../lib/crypto'
import { uploadPublicKey } from './keys'
import type { AuthResponse } from '../types'

const REFRESH_TOKEN_KEY = 'refresh_token'

export function getRefreshToken(): string | null {
  return localStorage.getItem(REFRESH_TOKEN_KEY)
}

function setRefreshToken(token: string | null) {
  if (token) localStorage.setItem(REFRESH_TOKEN_KEY, token)
  else localStorage.removeItem(REFRESH_TOKEN_KEY)
}

export async function publishKey(username: string): Promise<void> {
  try {
    const { keyPair } = await getOrCreateKeyPair(username)
    const pubKey = await exportPublicKey(keyPair)
    await uploadPublicKey(pubKey)
  } catch (err) {
    console.warn('[E2E] Could not upload public key:', err)
  }
}

export async function login(username: string, password: string): Promise<AuthResponse> {
  const data = await api.post<AuthResponse>('/api/v1/login', { username, password })
  setToken(data.token)
  setRefreshToken(data.refresh_token)
  await publishKey(data.username)
  return data
}

export async function register(username: string, password: string, inviteCode: string) {
  const data = await api.post<AuthResponse>('/api/v1/register', { username, password, invite_code: inviteCode })
  setToken(data.token)
  setRefreshToken(data.refresh_token)
  await publishKey(data.username)
  return data
}

// The backend rotates refresh tokens on every use and immediately deletes
// the old one — no grace window. refresh_token lives in localStorage, which
// is shared across tabs, so two tabs refreshing at once race: the loser's
// token has already been deleted by the winner and gets a hard 401. A
// cross-tab lock serializes refreshes so the loser waits, then re-reads the
// (now current) token instead of retrying with the stale one it started with.
async function doRefresh(): Promise<string | null> {
  const rt = getRefreshToken()
  if (!rt) return null
  try {
    const data = await api.post<{ token: string; refresh_token: string }>('/api/v1/refresh', { refresh_token: rt })
    setToken(data.token)
    setRefreshToken(data.refresh_token)
    return data.token
  } catch {
    // Only clear it if it's still the token we tried — don't wipe a
    // session another tab may have just rotated to successfully.
    if (getRefreshToken() === rt) setRefreshToken(null)
    return null
  }
}

export async function refresh(): Promise<string | null> {
  // Web Locks isn't implemented everywhere (e.g. jsdom in tests) — degrade
  // to the unlocked (racy) path there instead of throwing.
  if (!('locks' in navigator)) return doRefresh()
  return navigator.locks.request('onyxchat-refresh-token', doRefresh)
}

export async function logout(): Promise<void> {
  const rt = getRefreshToken()
  if (rt) {
    try {
      await api.post('/api/v1/logout', { refresh_token: rt })
    } catch {
      // best-effort — still clear local state
    }
  }
  const storedUser = localStorage.getItem('user')
  setToken(null)
  setRefreshToken(null)
  localStorage.removeItem('user')
  if (storedUser) {
    try {
      const { username } = JSON.parse(storedUser) as { username: string }
      await clearKeyPair(username)
    } catch {
      // malformed 'user' entry — nothing to clear by username
    }
  }
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  await api.patch('/api/v1/users/me/password', {
    current_password: currentPassword,
    new_password: newPassword,
  })
}
