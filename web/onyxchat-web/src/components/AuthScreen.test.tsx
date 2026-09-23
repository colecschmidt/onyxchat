// @vitest-environment jsdom

import { vi, describe, it, expect, afterEach, beforeEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AuthScreen } from './AuthScreen'

const mockLogin = vi.fn()
const mockRegister = vi.fn()
const mockCompleteTwoFactor = vi.fn()
const mockCancelTwoFactor = vi.fn()

// mfaRequired lives on a mutable object because the component reads it from
// context on every render — flipping it inside a mocked login is how a real
// challenge would arrive.
const authState = { mfaRequired: false }

vi.mock('../context/AuthContext', () => ({
  useAuth: () => ({
    login: mockLogin,
    register: mockRegister,
    completeTwoFactor: mockCompleteTwoFactor,
    cancelTwoFactor: mockCancelTwoFactor,
    mfaRequired: authState.mfaRequired,
  }),
}))

// A password-only account: login resolves without a challenge.
function loginSucceeds() {
  mockLogin.mockResolvedValue({ mfaRequired: false })
}

// An account with a second factor: the password step raises the challenge.
function loginNeedsSecondFactor() {
  mockLogin.mockImplementation(async () => {
    authState.mfaRequired = true
    return { mfaRequired: true }
  })
}

async function signIn(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByPlaceholderText('your username'), 'alice')
  await user.type(screen.getByPlaceholderText('••••••••'), 'hunter22')
  await user.click(submitButton())
}

// On the login tab, the tab button and the submit button both read "Sign in" —
// the submit button is always the last one rendered.
function submitButton() {
  const matches = screen.getAllByRole('button', { name: 'Sign in' })
  return matches[matches.length - 1]
}

describe('AuthScreen', () => {
  afterEach(() => cleanup())
  beforeEach(() => {
    vi.clearAllMocks()
    authState.mfaRequired = false
  })

  it('renders the sign-in tab by default with no invite code field', () => {
    render(<AuthScreen />)
    expect(screen.getAllByRole('button', { name: 'Sign in' })).toHaveLength(2)
    expect(screen.getByPlaceholderText('your username')).toBeTruthy()
    expect(screen.queryByPlaceholderText('xxxx-xxxx-xxxx')).toBeNull()
  })

  it('shows the invite code field and privacy policy link on the register tab', async () => {
    const user = userEvent.setup()
    render(<AuthScreen />)

    await user.click(screen.getByRole('button', { name: 'Register' }))

    expect(screen.getByPlaceholderText('xxxx-xxxx-xxxx')).toBeTruthy()
    expect(screen.getAllByRole('link', { name: 'Privacy Policy' }).length).toBeGreaterThan(0)
  })

  it('blocks submit with a validation error when fields are empty', async () => {
    const user = userEvent.setup()
    render(<AuthScreen />)

    await user.click(submitButton())

    expect(await screen.findByText('Username and password required.')).toBeTruthy()
    expect(mockLogin).not.toHaveBeenCalled()
  })

  it('blocks register when password is under 8 characters', async () => {
    const user = userEvent.setup()
    render(<AuthScreen />)

    await user.click(screen.getByRole('button', { name: 'Register' }))
    await user.type(screen.getByPlaceholderText('your username'), 'bob')
    await user.type(screen.getByPlaceholderText('min 8 characters'), 'short')
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    expect(await screen.findByText('Password must be at least 8 characters.')).toBeTruthy()
    expect(mockRegister).not.toHaveBeenCalled()
  })

  it('calls login with username and password on submit', async () => {
    loginSucceeds()
    const user = userEvent.setup()
    render(<AuthScreen />)

    await user.type(screen.getByPlaceholderText('your username'), 'alice')
    await user.type(screen.getByPlaceholderText('••••••••'), 'hunter22')
    await user.click(submitButton())

    expect(mockLogin).toHaveBeenCalledWith('alice', 'hunter22')
  })

  it('calls register with username, password, and invite code on submit', async () => {
    mockRegister.mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<AuthScreen />)

    await user.click(screen.getByRole('button', { name: 'Register' }))
    await user.type(screen.getByPlaceholderText('your username'), 'alice')
    await user.type(screen.getByPlaceholderText('min 8 characters'), 'hunter222')
    await user.type(screen.getByPlaceholderText('xxxx-xxxx-xxxx'), 'ab12-cd34-ef56')
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    expect(mockRegister).toHaveBeenCalledWith('alice', 'hunter222', 'ab12-cd34-ef56')
  })

  it('surfaces the thrown error message when login fails', async () => {
    mockLogin.mockRejectedValue(new Error('invalid credentials'))
    const user = userEvent.setup()
    render(<AuthScreen />)

    await user.type(screen.getByPlaceholderText('your username'), 'alice')
    await user.type(screen.getByPlaceholderText('••••••••'), 'hunter22')
    await user.click(submitButton())

    expect(await screen.findByText('invalid credentials')).toBeTruthy()
  })

  it('submits on Enter key press in a field', async () => {
    loginSucceeds()
    const user = userEvent.setup()
    render(<AuthScreen />)

    await user.type(screen.getByPlaceholderText('your username'), 'alice')
    await user.type(screen.getByPlaceholderText('••••••••'), 'hunter22{enter}')

    expect(mockLogin).toHaveBeenCalledWith('alice', 'hunter22')
  })

  it('clears the error when switching tabs', async () => {
    const user = userEvent.setup()
    render(<AuthScreen />)

    await user.click(submitButton())
    expect(await screen.findByText('Username and password required.')).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Register' }))
    expect(screen.queryByText('Username and password required.')).toBeNull()
  })

  // ── Two-factor step ────────────────────────────────────────────────────────

  it('moves to the verification step when the account has a second factor', async () => {
    loginNeedsSecondFactor()
    const user = userEvent.setup()
    render(<AuthScreen />)

    await signIn(user)

    expect(await screen.findByText('Two-factor verification')).toBeTruthy()
    // The password form is gone, so nothing can be resubmitted behind the code.
    expect(screen.queryByPlaceholderText('your username')).toBeNull()
  })

  it('sends the typed code to completeTwoFactor', async () => {
    authState.mfaRequired = true
    mockCompleteTwoFactor.mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<AuthScreen />)

    await user.type(screen.getByPlaceholderText('123456'), '424242')
    await user.click(screen.getByRole('button', { name: 'Verify' }))

    expect(mockCompleteTwoFactor).toHaveBeenCalledWith('424242')
  })

  it('verifies on Enter in the code field', async () => {
    authState.mfaRequired = true
    mockCompleteTwoFactor.mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<AuthScreen />)

    await user.type(screen.getByPlaceholderText('123456'), '424242{enter}')

    expect(mockCompleteTwoFactor).toHaveBeenCalledWith('424242')
  })

  it('refuses to submit an empty code', async () => {
    authState.mfaRequired = true
    const user = userEvent.setup()
    render(<AuthScreen />)

    await user.click(screen.getByRole('button', { name: 'Verify' }))

    expect(await screen.findByText('Enter the code from your authenticator app.')).toBeTruthy()
    expect(mockCompleteTwoFactor).not.toHaveBeenCalled()
  })

  it('surfaces a rejected code and clears the field for another try', async () => {
    authState.mfaRequired = true
    mockCompleteTwoFactor.mockRejectedValue(new Error('invalid verification code'))
    const user = userEvent.setup()
    render(<AuthScreen />)

    const field = screen.getByPlaceholderText('123456') as HTMLInputElement
    await user.type(field, '000000')
    await user.click(screen.getByRole('button', { name: 'Verify' }))

    expect(await screen.findByText('invalid verification code')).toBeTruthy()
    expect(field.value).toBe('')
  })

  it('swaps to recovery-code entry on request', async () => {
    authState.mfaRequired = true
    const user = userEvent.setup()
    render(<AuthScreen />)

    await user.click(screen.getByRole('button', { name: 'Use a recovery code' }))

    expect(screen.getByPlaceholderText('xxxx-xxxx-xxxx-xxxx')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Use your authenticator app' })).toBeTruthy()
  })

  it('abandons the challenge when going back to sign in', async () => {
    authState.mfaRequired = true
    const user = userEvent.setup()
    render(<AuthScreen />)

    await user.click(screen.getByRole('button', { name: 'Back to sign in' }))

    expect(mockCancelTwoFactor).toHaveBeenCalledOnce()
  })
})
