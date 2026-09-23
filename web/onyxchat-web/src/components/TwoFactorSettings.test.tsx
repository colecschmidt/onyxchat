// @vitest-environment jsdom

import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TwoFactorSettings } from './TwoFactorSettings'

vi.mock('../api/twofactor', () => ({
  getTwoFactorStatus:      vi.fn(),
  startTwoFactorSetup:     vi.fn(),
  confirmTwoFactorSetup:   vi.fn(),
  disableTwoFactor:        vi.fn(),
  regenerateRecoveryCodes: vi.fn(),
}))

import * as api from '../api/twofactor'

const OFF = { enabled: false, pending_setup: false, recovery_codes_remaining: 0 }
const ON = { enabled: true, pending_setup: false, recovery_codes_remaining: 10 }

const SETUP = {
  secret: 'JBSWY3DPEHPK3PXP',
  otpauth_uri: 'otpauth://totp/OnyxChat:alice?secret=JBSWY3DPEHPK3PXP&issuer=OnyxChat',
  qr_code: 'data:image/png;base64,iVBORw0KGgo=',
}

const CODES = Array.from({ length: 10 }, (_, i) => `aaaa-bbbb-cccc-000${i}`)

async function renderPanel(status = OFF) {
  vi.mocked(api.getTwoFactorStatus).mockResolvedValue(status)
  render(<TwoFactorSettings username="alice" />)
  await screen.findByText('Authenticator app')
}

describe('TwoFactorSettings', () => {
  afterEach(() => cleanup())
  beforeEach(() => vi.clearAllMocks())

  it('reports the factor as off for an account without one', async () => {
    await renderPanel(OFF)

    expect(screen.getByText('OFF')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Set up authenticator app' })).toBeTruthy()
  })

  it('reports the factor as on, with the recovery codes left', async () => {
    await renderPanel(ON)

    expect(screen.getByText('ON')).toBeTruthy()
    expect(screen.getByText(/10 recovery codes left/)).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Turn off' })).toBeTruthy()
  })

  it('warns when the recovery codes are nearly used up', async () => {
    await renderPanel({ enabled: true, pending_setup: false, recovery_codes_remaining: 2 })

    expect(screen.getByText(/You have 2 recovery/)).toBeTruthy()
  })

  it('asks for the password before issuing a secret', async () => {
    const user = userEvent.setup()
    await renderPanel(OFF)

    await user.click(screen.getByRole('button', { name: 'Set up authenticator app' }))
    await user.click(screen.getByRole('button', { name: 'Continue' }))

    expect(await screen.findByText('Enter your password to continue.')).toBeTruthy()
    expect(api.startTwoFactorSetup).not.toHaveBeenCalled()
  })

  it('shows the QR and the manual key, then the recovery codes on confirm', async () => {
    const user = userEvent.setup()
    vi.mocked(api.startTwoFactorSetup).mockResolvedValue(SETUP)
    vi.mocked(api.confirmTwoFactorSetup).mockResolvedValue({ recovery_codes: CODES })
    await renderPanel(OFF)

    await user.click(screen.getByRole('button', { name: 'Set up authenticator app' }))
    await user.type(screen.getByPlaceholderText('••••••••'), 'hunter22')
    await user.click(screen.getByRole('button', { name: 'Continue' }))

    expect(api.startTwoFactorSetup).toHaveBeenCalledWith('hunter22')

    const qr = await screen.findByAltText('Two-factor setup QR code')
    expect(qr.getAttribute('src')).toBe(SETUP.qr_code)
    expect(screen.getByText(SETUP.secret)).toBeTruthy()

    // Status is refreshed after confirmation so the panel reflects the new state.
    vi.mocked(api.getTwoFactorStatus).mockResolvedValue(ON)

    await user.type(screen.getByPlaceholderText('123456'), '424242')
    await user.click(screen.getByRole('button', { name: 'Verify and enable' }))

    expect(api.confirmTwoFactorSetup).toHaveBeenCalledWith('424242')
    expect(await screen.findByText('Save your recovery codes')).toBeTruthy()
    for (const c of CODES) {
      expect(screen.getByText(c)).toBeTruthy()
    }
  })

  it('leaves the factor off when the confirmation code is rejected', async () => {
    const user = userEvent.setup()
    vi.mocked(api.startTwoFactorSetup).mockResolvedValue(SETUP)
    vi.mocked(api.confirmTwoFactorSetup).mockRejectedValue(new Error('invalid verification code'))
    await renderPanel(OFF)

    await user.click(screen.getByRole('button', { name: 'Set up authenticator app' }))
    await user.type(screen.getByPlaceholderText('••••••••'), 'hunter22')
    await user.click(screen.getByRole('button', { name: 'Continue' }))

    await screen.findByAltText('Two-factor setup QR code')
    await user.type(screen.getByPlaceholderText('123456'), '000000')
    await user.click(screen.getByRole('button', { name: 'Verify and enable' }))

    expect(await screen.findByText('invalid verification code')).toBeTruthy()
    expect(screen.queryByText('Save your recovery codes')).toBeNull()
  })

  it('requires both a password and a code to turn the factor off', async () => {
    const user = userEvent.setup()
    vi.mocked(api.disableTwoFactor).mockResolvedValue(undefined)
    await renderPanel(ON)

    await user.click(screen.getByRole('button', { name: 'Turn off' }))

    // Password only.
    await user.type(screen.getByPlaceholderText('••••••••'), 'hunter22')
    await user.click(screen.getByRole('button', { name: 'Turn off' }))
    expect(await screen.findByText(/Enter a code from your authenticator app/)).toBeTruthy()
    expect(api.disableTwoFactor).not.toHaveBeenCalled()

    // Both.
    vi.mocked(api.getTwoFactorStatus).mockResolvedValue(OFF)
    await user.type(screen.getByPlaceholderText('123456'), '424242')
    await user.click(screen.getByRole('button', { name: 'Turn off' }))

    await waitFor(() => expect(api.disableTwoFactor).toHaveBeenCalledWith('hunter22', '424242'))
    expect(await screen.findByText('Two-factor authentication is off.')).toBeTruthy()
  })

  it('accepts a recovery code in place of an authenticator code', async () => {
    const user = userEvent.setup()
    vi.mocked(api.disableTwoFactor).mockResolvedValue(undefined)
    await renderPanel(ON)

    await user.click(screen.getByRole('button', { name: 'Turn off' }))
    await user.type(screen.getByPlaceholderText('••••••••'), 'hunter22')
    await user.type(screen.getByPlaceholderText('123456'), CODES[0])

    vi.mocked(api.getTwoFactorStatus).mockResolvedValue(OFF)
    await user.click(screen.getByRole('button', { name: 'Turn off' }))

    await waitFor(() => expect(api.disableTwoFactor).toHaveBeenCalledWith('hunter22', CODES[0]))
  })

  it('shows a fresh set of codes after regenerating', async () => {
    const user = userEvent.setup()
    const fresh = CODES.map(c => c.replace('aaaa', 'zzzz'))
    vi.mocked(api.regenerateRecoveryCodes).mockResolvedValue({ recovery_codes: fresh })
    await renderPanel(ON)

    await user.click(screen.getByRole('button', { name: 'New recovery codes' }))
    await user.type(screen.getByPlaceholderText('••••••••'), 'hunter22')
    await user.type(screen.getByPlaceholderText('123456'), '424242')
    await user.click(screen.getByRole('button', { name: 'Generate' }))

    await waitFor(() => expect(api.regenerateRecoveryCodes).toHaveBeenCalledWith('hunter22', '424242'))
    expect(await screen.findByText('Save your recovery codes')).toBeTruthy()
    expect(screen.getByText(fresh[0])).toBeTruthy()
  })

  it('abandoning setup leaves the factor untouched', async () => {
    const user = userEvent.setup()
    vi.mocked(api.startTwoFactorSetup).mockResolvedValue(SETUP)
    await renderPanel(OFF)

    await user.click(screen.getByRole('button', { name: 'Set up authenticator app' }))
    await user.type(screen.getByPlaceholderText('••••••••'), 'hunter22')
    await user.click(screen.getByRole('button', { name: 'Continue' }))

    await screen.findByAltText('Two-factor setup QR code')
    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(await screen.findByText('OFF')).toBeTruthy()
    expect(api.confirmTwoFactorSetup).not.toHaveBeenCalled()
  })
})
