import { api } from './client'
import type { RecoveryCodesResponse, TwoFactorStatus, TwoFactorSetup } from '../types'

// Enrollment is a two-step commit: startSetup stores a *pending* secret and
// hands back the QR, and only confirmSetup — which proves the authenticator
// actually holds that secret — turns the factor on. Abandoning setup halfway
// therefore can't lock anyone out.

export function getTwoFactorStatus(): Promise<TwoFactorStatus> {
  return api.get<TwoFactorStatus>('/api/v1/users/me/2fa')
}

export function startTwoFactorSetup(password: string): Promise<TwoFactorSetup> {
  return api.post<TwoFactorSetup>('/api/v1/users/me/2fa/setup', { password })
}

export function confirmTwoFactorSetup(code: string): Promise<RecoveryCodesResponse> {
  return api.post<RecoveryCodesResponse>('/api/v1/users/me/2fa/confirm', { code: code.trim() })
}

export function disableTwoFactor(password: string, code: string): Promise<void> {
  return api.delete('/api/v1/users/me/2fa', { password, code: code.trim() })
}

export function regenerateRecoveryCodes(password: string, code: string): Promise<RecoveryCodesResponse> {
  return api.post<RecoveryCodesResponse>('/api/v1/users/me/2fa/recovery-codes', {
    password,
    code: code.trim(),
  })
}
