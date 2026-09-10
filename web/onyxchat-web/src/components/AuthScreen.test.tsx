// @vitest-environment jsdom

import { vi, describe, it, expect, afterEach, beforeEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AuthScreen } from './AuthScreen'

const mockLogin = vi.fn()
const mockRegister = vi.fn()

vi.mock('../context/AuthContext', () => ({
  useAuth: () => ({
    login: mockLogin,
    register: mockRegister,
  }),
}))

// On the login tab, the tab button and the submit button both read "Sign in" —
// the submit button is always the last one rendered.
function submitButton() {
  const matches = screen.getAllByRole('button', { name: 'Sign in' })
  return matches[matches.length - 1]
}

describe('AuthScreen', () => {
  afterEach(() => cleanup())
  beforeEach(() => vi.clearAllMocks())

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
    mockLogin.mockResolvedValue(undefined)
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
    mockLogin.mockResolvedValue(undefined)
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
})
