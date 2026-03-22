// context/AuthContext.tsx — replaces existing file
// One change: logout is now async (calls clearKeyPair from crypto.ts via auth.ts)

import { createContext, useContext, useState, useEffect, type ReactNode } from 'react'
import { login, logout as apiLogout, register } from '../api/auth'
import type { User } from '../types'

interface AuthState {
  user: User | null
  isAuthenticated: boolean
  login:    (username: string, password: string) => Promise<void>
  register: (username: string, password: string, inviteCode: string) => Promise<void>
  logout:   () => Promise<void>
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(() => {
    const stored = sessionStorage.getItem('user')
    return stored ? JSON.parse(stored) : null
  })

  useEffect(() => {
    if (user) sessionStorage.setItem('user', JSON.stringify(user))
  }, [user])

  async function handleLogin(username: string, password: string) {
    const data = await login(username, password)
    setUser({ id: data.id, username: data.username })
  }

  async function handleRegister(username: string, password: string, inviteCode: string) {
    const data = await register(username, password, inviteCode)
    setUser({ id: data.id, username: data.username })
  }

  async function handleLogout() {
    await apiLogout()   // clears token + IDB keypair
    setUser(null)
  }

  return (
    <AuthContext.Provider value={{
      user,
      isAuthenticated: !!user,
      login:    handleLogin,
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