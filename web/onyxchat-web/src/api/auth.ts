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

export async function publishKey(): Promise<void> {
  try {
    const kp     = await getOrCreateKeyPair()
    const pubKey = await exportPublicKey(kp)
    await uploadPublicKey(pubKey)
  } catch (err) {
    console.warn('[E2E] Could not upload public key:', err)
  }
}

export async function login(username: string, password: string): Promise<AuthResponse> {
  const data = await api.post<AuthResponse>('/api/v1/login', { username, password })
  setToken(data.token)
  setRefreshToken(data.refresh_token)
  await publishKey()
  return data
}

export async function register(username: string, password: string, inviteCode: string) {
  const data = await api.post<AuthResponse>('/api/v1/register', { username, password, invite_code: inviteCode })
  setToken(data.token)
  setRefreshToken(data.refresh_token)
  await publishKey()
  return data
}

export async function refresh(): Promise<string | null> {
  const rt = getRefreshToken()
  if (!rt) return null
  try {
    const data = await api.post<{ token: string; refresh_token: string }>('/api/v1/refresh', { refresh_token: rt })
    setToken(data.token)
    setRefreshToken(data.refresh_token)
    return data.token
  } catch {
    setRefreshToken(null)
    return null
  }
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
  setToken(null)
  setRefreshToken(null)
  localStorage.removeItem('user')
  await clearKeyPair()
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  await api.patch('/api/v1/users/me/password', {
    current_password: currentPassword,
    new_password: newPassword,
  })
}
