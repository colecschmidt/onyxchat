// @vitest-environment jsdom

import { vi, describe, it, expect, afterEach, beforeEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import App from './App'

const mockUseAuth = vi.fn()

vi.mock('./context/AuthContext', () => ({
  useAuth: () => mockUseAuth(),
}))
vi.mock('./components/AuthScreen', () => ({
  AuthScreen: () => <div data-testid="auth-screen" />,
}))
vi.mock('./components/Sidebar', () => ({
  Sidebar: () => <div data-testid="sidebar" />,
}))
vi.mock('./components/ChatPanel', () => ({
  ChatPanel: () => <div data-testid="chat-panel" />,
}))
vi.mock('./components/AdminPanel', () => ({
  AdminPanel: () => <div data-testid="admin-panel" />,
}))

function setPath(path: string) {
  window.history.pushState({}, '', path)
}

describe('App routing', () => {
  afterEach(() => cleanup())
  beforeEach(() => {
    vi.clearAllMocks()
    setPath('/')
  })

  it('shows AuthScreen when logged out', () => {
    mockUseAuth.mockReturnValue({ isAuthenticated: false })
    render(<App />)
    expect(screen.getByTestId('auth-screen')).toBeTruthy()
  })

  it('shows Sidebar + ChatPanel when logged in at root path', () => {
    mockUseAuth.mockReturnValue({ isAuthenticated: true })
    render(<App />)
    expect(screen.getByTestId('sidebar')).toBeTruthy()
    expect(screen.getByTestId('chat-panel')).toBeTruthy()
  })

  it('shows AuthScreen on /admin when logged out, not AdminPanel', () => {
    setPath('/admin')
    mockUseAuth.mockReturnValue({ isAuthenticated: false })
    render(<App />)
    expect(screen.getByTestId('auth-screen')).toBeTruthy()
    expect(screen.queryByTestId('admin-panel')).toBeNull()
  })

  it('shows AdminPanel on /admin when logged in', () => {
    setPath('/admin')
    mockUseAuth.mockReturnValue({ isAuthenticated: true })
    render(<App />)
    expect(screen.getByTestId('admin-panel')).toBeTruthy()
    expect(screen.queryByTestId('sidebar')).toBeNull()
  })
})
