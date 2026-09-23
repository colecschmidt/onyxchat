// context/AuthContext.tsx
// logout is async (calls clearKeyPair from crypto.ts via auth.ts).
// login may not finish the sign-in: an account with a second factor stops at a
// challenge, and completeTwoFactor finishes it.

import { createContext, useContext, useState, useEffect, type ReactNode } from 'react'
import {
  login,
  logout as apiLogout,
  register,
  refresh as apiRefresh,
  getRefreshToken,
  publishKey,
  completeTwoFactorLogin,
} from '../api/auth'
import { getToken } from '../api/client'
import type { User } from '../types'

interface AuthState {
  user: User | null
  isAuthenticated: boolean
  /** True between a correct password and a verified second factor. */
  mfaRequired: boolean
  login:    (username: string, password: string) => Promise<{ mfaRequired: boolean }>
  completeTwoFactor: (code: string) => Promise<void>
  cancelTwoFactor: () => void
  register: (username: string, password: string, inviteCode: string) => Promise<void>
  logout:   () => Promise<void>
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(() => {
    const stored = localStorage.getItem('user')
    return stored ? JSON.parse(stored) : null
  })

  // The pending challenge token. Deliberately component state rather than
  // localStorage: it is valid for five minutes, is useless without the code,
  // and should not outlive the tab that started the sign-in.
  const [mfaToken, setMfaToken] = useState<string | null>(null)

  useEffect(() => {
    if (user) localStorage.setItem('user', JSON.stringify(user))
    else localStorage.removeItem('user')
  }, [user])

  // On mount: if we have a stored refresh token but lost the session token,
  // silently get a new access token so the user doesn't have to log in again.
  // Refresh tokens are only ever issued after a second factor has been
  // verified, so this path does not need to re-challenge.
  useEffect(() => {
    if (getToken() || !getRefreshToken()) return

    let mounted = true
    // Read directly from storage rather than closing over the `user` state
    // value — this only needs the value as of mount, not on every change.
    const stored = localStorage.getItem('user')
    const storedUser: User | null = stored ? JSON.parse(stored) : null

    apiRefresh()
      .then(async newToken => {
        if (!mounted) return
        if (!newToken || !storedUser) { setUser(null); return }
        await publishKey(storedUser.username)
      })
      .catch(() => {
        if (!mounted) return
        setUser(null)
      })

    return () => { mounted = false }
  }, [])

  async function handleLogin(username: string, password: string) {
    const result = await login(username, password)

    if (result.status === 'mfa_required') {
      setMfaToken(result.mfaToken)
      return { mfaRequired: true }
    }

    setMfaToken(null)
    setUser({ id: result.auth.id, username: result.auth.username })
    return { mfaRequired: false }
  }

  async function handleCompleteTwoFactor(code: string) {
    if (!mfaToken) throw new Error('This sign-in has expired. Please start again.')

    const data = await completeTwoFactorLogin(mfaToken, code)
    setMfaToken(null)
    setUser({ id: data.id, username: data.username })
  }

  function handleCancelTwoFactor() {
    setMfaToken(null)
  }

  async function handleRegister(username: string, password: string, inviteCode: string) {
    const data = await register(username, password, inviteCode)
    setUser({ id: data.id, username: data.username })
  }

  async function handleLogout() {
    await apiLogout()   // clears token + IDB keypair
    setMfaToken(null)
    setUser(null)
  }

  return (
    <AuthContext.Provider value={{
      user,
      isAuthenticated: !!user,
      mfaRequired: !!mfaToken,
      login:    handleLogin,
      completeTwoFactor: handleCompleteTwoFactor,
      cancelTwoFactor:   handleCancelTwoFactor,
      register: handleRegister,
      logout:   handleLogout,
    }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
