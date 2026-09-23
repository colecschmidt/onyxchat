// @vitest-environment jsdom

import { useEffect } from 'react'
import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest'
import { render, screen, act, cleanup } from '@testing-library/react'
import { AuthProvider, useAuth } from './AuthContext'

vi.mock('../api/auth', () => ({
  refresh:         vi.fn(),
  getRefreshToken: vi.fn(),
  publishKey:      vi.fn(),
  login:           vi.fn(),
  logout:          vi.fn(),
  register:        vi.fn(),
  completeTwoFactorLogin: vi.fn(),
}))
vi.mock('../api/client', () => ({
  getToken:  vi.fn(),
  setToken:  vi.fn(),
  api:       { post: vi.fn() },
}))

import * as authApi   from '../api/auth'
import * as clientApi from '../api/client'

function Probe() {
  const { user } = useAuth()
  return <div data-testid="probe">{user?.username ?? 'logged-out'}</div>
}

describe('AuthContext – mount-time session refresh', () => {
  afterEach(() => cleanup())

  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('calls publishKey and keeps user when refresh succeeds', async () => {
    localStorage.setItem('user', JSON.stringify({ id: 1, username: 'alice' }))
    vi.mocked(clientApi.getToken).mockReturnValue(null)
    vi.mocked(authApi.getRefreshToken).mockReturnValue('rt')
    vi.mocked(authApi.refresh).mockResolvedValue('new-token')
    vi.mocked(authApi.publishKey).mockResolvedValue(undefined)

    await act(async () => {
      render(<AuthProvider><Probe /></AuthProvider>)
    })

    expect(authApi.refresh).toHaveBeenCalledOnce()
    expect(authApi.publishKey).toHaveBeenCalledOnce()
    expect(screen.getByTestId('probe').textContent).toBe('alice')
  })

  it('clears user when refresh returns null (token expired)', async () => {
    localStorage.setItem('user', JSON.stringify({ id: 1, username: 'alice' }))
    vi.mocked(clientApi.getToken).mockReturnValue(null)
    vi.mocked(authApi.getRefreshToken).mockReturnValue('rt')
    vi.mocked(authApi.refresh).mockResolvedValue(null)

    await act(async () => {
      render(<AuthProvider><Probe /></AuthProvider>)
    })

    expect(screen.getByTestId('probe').textContent).toBe('logged-out')
    expect(authApi.publishKey).not.toHaveBeenCalled()
  })

  it('clears user and does not call publishKey when refresh throws', async () => {
    localStorage.setItem('user', JSON.stringify({ id: 1, username: 'alice' }))
    vi.mocked(clientApi.getToken).mockReturnValue(null)
    vi.mocked(authApi.getRefreshToken).mockReturnValue('rt')
    vi.mocked(authApi.refresh).mockRejectedValue(new Error('network error'))

    await act(async () => {
      render(<AuthProvider><Probe /></AuthProvider>)
    })

    expect(screen.getByTestId('probe').textContent).toBe('logged-out')
    expect(authApi.publishKey).not.toHaveBeenCalled()
  })

  it('skips refresh when an access token is already in memory', async () => {
    vi.mocked(clientApi.getToken).mockReturnValue('still-valid-token')
    vi.mocked(authApi.getRefreshToken).mockReturnValue('rt')

    await act(async () => {
      render(<AuthProvider><Probe /></AuthProvider>)
    })

    expect(authApi.refresh).not.toHaveBeenCalled()
  })

  it('skips refresh when no refresh token is stored', async () => {
    vi.mocked(clientApi.getToken).mockReturnValue(null)
    vi.mocked(authApi.getRefreshToken).mockReturnValue(null)

    await act(async () => {
      render(<AuthProvider><Probe /></AuthProvider>)
    })

    expect(authApi.refresh).not.toHaveBeenCalled()
  })

  it('does not call setState after unmount (mounted guard)', async () => {
    localStorage.setItem('user', JSON.stringify({ id: 1, username: 'alice' }))
    vi.mocked(clientApi.getToken).mockReturnValue(null)
    vi.mocked(authApi.getRefreshToken).mockReturnValue('rt')

    // Refresh won't resolve until we say so
    let resolveRefresh!: (token: string) => void
    vi.mocked(authApi.refresh).mockReturnValue(
      new Promise<string>(r => { resolveRefresh = r })
    )

    let unmount!: () => void
    await act(async () => {
      const result = render(<AuthProvider><Probe /></AuthProvider>)
      unmount = result.unmount
    })

    // Unmount before refresh resolves
    act(() => unmount())

    // Resolving after unmount must not throw or trigger a React warning.
    // Without the mounted guard this would call setUser on a dead component.
    await act(async () => { resolveRefresh('late-token') })
  })
})

// ─────────────────────────────────────────────────────────────
// Two-factor challenges
// ─────────────────────────────────────────────────────────────

// Exposes the pieces of the context the challenge flow touches, so a test can
// drive login/completeTwoFactor and read back what the provider decided.
// The context is published to the test from an effect rather than during
// render — writing to an outer binding mid-render is what
// react-hooks/immutability exists to catch. Every act() in these tests flushes
// effects, so the handle is current by the time a test reads it.
const probeRef: { current?: ReturnType<typeof useAuth> } = {}
const auth = () => probeRef.current!

function ChallengeProbe() {
  const ctx = useAuth()
  useEffect(() => { probeRef.current = ctx })
  return (
    <div>
      <span data-testid="user">{ctx.user?.username ?? 'logged-out'}</span>
      <span data-testid="mfa">{String(ctx.mfaRequired)}</span>
    </div>
  )
}

describe('AuthContext – two-factor challenge', () => {
  afterEach(() => cleanup())

  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
    // No stored session, so the mount-time refresh effect stays out of the way.
    vi.mocked(clientApi.getToken).mockReturnValue(null)
    vi.mocked(authApi.getRefreshToken).mockReturnValue(null)
  })

  async function mount() {
    await act(async () => {
      render(<AuthProvider><ChallengeProbe /></AuthProvider>)
    })
  }

  it('signs in directly when the account has no second factor', async () => {
    vi.mocked(authApi.login).mockResolvedValue({
      status: 'ok',
      auth: { id: 1, username: 'alice', token: 't', refresh_token: 'rt' },
    })
    await mount()

    let result!: { mfaRequired: boolean }
    await act(async () => { result = await auth().login('alice', 'pw') })

    expect(result.mfaRequired).toBe(false)
    expect(screen.getByTestId('user').textContent).toBe('alice')
    expect(screen.getByTestId('mfa').textContent).toBe('false')
  })

  it('does not sign the user in when a challenge is returned', async () => {
    vi.mocked(authApi.login).mockResolvedValue({ status: 'mfa_required', mfaToken: 'chal-1' })
    await mount()

    let result!: { mfaRequired: boolean }
    await act(async () => { result = await auth().login('alice', 'pw') })

    expect(result.mfaRequired).toBe(true)
    expect(screen.getByTestId('mfa').textContent).toBe('true')
    // The password alone must not produce a session.
    expect(screen.getByTestId('user').textContent).toBe('logged-out')
    expect(localStorage.getItem('user')).toBeNull()
  })

  it('completes the sign-in with a verified code', async () => {
    vi.mocked(authApi.login).mockResolvedValue({ status: 'mfa_required', mfaToken: 'chal-1' })
    vi.mocked(authApi.completeTwoFactorLogin).mockResolvedValue({
      id: 1, username: 'alice', token: 't', refresh_token: 'rt',
    })
    await mount()

    await act(async () => { await auth().login('alice', 'pw') })
    await act(async () => { await auth().completeTwoFactor('424242') })

    expect(authApi.completeTwoFactorLogin).toHaveBeenCalledWith('chal-1', '424242')
    expect(screen.getByTestId('user').textContent).toBe('alice')
    expect(screen.getByTestId('mfa').textContent).toBe('false')
  })

  it('keeps the challenge open when the code is rejected', async () => {
    vi.mocked(authApi.login).mockResolvedValue({ status: 'mfa_required', mfaToken: 'chal-1' })
    vi.mocked(authApi.completeTwoFactorLogin).mockRejectedValue(new Error('invalid verification code'))
    await mount()

    await act(async () => { await auth().login('alice', 'pw') })
    await act(async () => {
      await expect(auth().completeTwoFactor('000000')).rejects.toThrow('invalid verification code')
    })

    expect(screen.getByTestId('mfa').textContent).toBe('true')
    expect(screen.getByTestId('user').textContent).toBe('logged-out')
  })

  it('refuses to verify a code with no challenge in flight', async () => {
    await mount()

    await act(async () => {
      await expect(auth().completeTwoFactor('424242')).rejects.toThrow(/expired/)
    })
    expect(authApi.completeTwoFactorLogin).not.toHaveBeenCalled()
  })

  it('drops the challenge on cancel', async () => {
    vi.mocked(authApi.login).mockResolvedValue({ status: 'mfa_required', mfaToken: 'chal-1' })
    await mount()

    await act(async () => { await auth().login('alice', 'pw') })
    expect(screen.getByTestId('mfa').textContent).toBe('true')

    act(() => auth().cancelTwoFactor())
    expect(screen.getByTestId('mfa').textContent).toBe('false')
  })
})
