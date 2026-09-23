// types/index.ts — replaces existing file
// Added: iv, encrypted to Message; E2E key API types

export interface User {
  id: number
  username: string
}

export interface Message {
  id: number
  senderId: number
  recipientId: number
  body: string
  createdAt: string
  // E2E fields — present when encrypted === true
  iv?: string
  encrypted: boolean
  // UI-only: true if send failed (never persisted to server)
  failed?: boolean
}

export interface Contact extends User {
  online: boolean
  lastMsg?: Message
}

export interface AuthResponse {
  id: number
  username: string
  token: string
  refresh_token: string
}

// What POST /api/v1/login actually returns: either a finished session or a
// two-factor challenge to answer at /api/v1/login/2fa. The two are mutually
// exclusive, hence every field being optional.
export interface LoginResponse extends Partial<AuthResponse> {
  mfa_required?: boolean
  mfa_token?: string
}

export type LoginResult =
  | { status: 'ok'; auth: AuthResponse }
  | { status: 'mfa_required'; mfaToken: string }

export interface TwoFactorStatus {
  enabled: boolean
  pending_setup: boolean
  recovery_codes_remaining: number
}

export interface TwoFactorSetup {
  secret: string       // base32, for manual entry
  otpauth_uri: string  // deep link for mobile authenticators
  qr_code: string      // PNG data URI
}

export interface RecoveryCodesResponse {
  recovery_codes: string[]
}

// WebSocket message types
export interface WSMessage {
  type: 'message' | 'typing' | 'presence' | 'key_changed' | 'message_deleted'
}

export interface WSChatMessage extends WSMessage {
  type: 'message'
  message: Message
}

export interface WSTyping extends WSMessage {
  type: 'typing'
  from: string
  to: string
  isTyping: boolean
}

export interface WSPresence extends WSMessage {
  type: 'presence'
  userId: number
  username: string
  status: 'online' | 'offline'
}

export interface WSKeyChanged extends WSMessage {
  type: 'key_changed'
  username: string
}

export interface WSMessageDeleted extends WSMessage {
  type: 'message_deleted'
  messageId: number
}

// E2E key API
export interface GetKeyResponse {
  username: string
  publicKey: string
}