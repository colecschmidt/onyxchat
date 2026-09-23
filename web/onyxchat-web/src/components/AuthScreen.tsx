import { useState } from 'react'
import { useAuth } from '../context/AuthContext'
import { Logo, Input, Button, StatusMessage } from './ui'

export function AuthScreen() {
  const { login, register, completeTwoFactor, cancelTwoFactor, mfaRequired } = useAuth()
  const [tab, setTab] = useState<'login' | 'register'>('login')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [inviteCode, setInviteCode] = useState('')

  // Second-factor step
  const [code, setCode] = useState('')
  const [useRecoveryCode, setUseRecoveryCode] = useState(false)

  async function handleSubmit() {
    setError('')
    if (!username || !password) return setError('Username and password required.')
    if (tab === 'register' && password.length < 8) return setError('Password must be at least 8 characters.')

    setLoading(true)
    try {
      if (tab === 'login') {
        const { mfaRequired } = await login(username, password)
        // On the 2FA branch nothing is signed in yet — the password field is
        // cleared so it isn't left sitting in the DOM during the code step.
        if (mfaRequired) setPassword('')
      } else {
        await register(username, password, inviteCode)
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setLoading(false)
    }
  }

  async function handleVerify() {
    setError('')
    if (!code.trim()) return setError('Enter the code from your authenticator app.')

    setLoading(true)
    try {
      await completeTwoFactor(code)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not verify that code.')
      setCode('')
    } finally {
      setLoading(false)
    }
  }

  function handleBackToSignIn() {
    cancelTwoFactor()
    setCode('')
    setUseRecoveryCode(false)
    setError('')
  }

  const linkButtonStyle = {
    background: 'transparent', border: 'none', padding: 0,
    fontSize: '12px', fontFamily: 'var(--font)',
    color: 'var(--text-mute)', cursor: 'pointer',
    textDecoration: 'underline',
  } as const

  return (
    <div style={{
      position: 'fixed', inset: 0,
      background: 'var(--bg)',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      padding: '20px',
      overflowY: 'auto',
    }}>
      <div style={{
        width: '100%', maxWidth: '360px',
        background: 'var(--bg-2)',
        border: '1px solid var(--border-2)',
        borderRadius: '20px',
        padding: '36px 32px',
      }}>
        {/* Logo */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '28px' }}>
          <Logo size={32} />
          <span style={{ fontSize: '20px', fontWeight: 700, letterSpacing: '-0.02em', color: 'var(--text)' }}>
            Onyx<span style={{ color: 'var(--accent)' }}>Chat</span>
          </span>
        </div>

        {mfaRequired ? (
          <>
            <h2 style={{
              fontSize: '16px', fontWeight: 600, color: 'var(--text)',
              margin: '0 0 6px',
            }}>
              Two-factor verification
            </h2>
            <p style={{ fontSize: '12px', color: 'var(--text-mute)', margin: '0 0 20px', lineHeight: 1.5 }}>
              {useRecoveryCode
                ? 'Enter one of the recovery codes you saved when you turned on two-factor authentication.'
                : 'Enter the 6-digit code from your authenticator app.'}
            </p>

            {error && <div style={{ marginBottom: '12px' }}><StatusMessage kind="error">{error}</StatusMessage></div>}

            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              <Input
                label={useRecoveryCode ? 'Recovery code' : 'Verification code'}
                value={code}
                onChange={e => setCode(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleVerify()}
                placeholder={useRecoveryCode ? 'xxxx-xxxx-xxxx-xxxx' : '123456'}
                autoComplete="one-time-code"
                autoFocus
                style={{ fontFamily: 'var(--mono)', letterSpacing: '0.08em' }}
              />

              <Button onClick={handleVerify} disabled={loading} fullWidth style={{ marginTop: '4px' }}>
                {loading ? 'Verifying…' : 'Verify'}
              </Button>

              <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: '8px' }}>
                <button
                  type="button"
                  onClick={() => { setUseRecoveryCode(v => !v); setCode(''); setError('') }}
                  style={linkButtonStyle}
                >
                  {useRecoveryCode ? 'Use your authenticator app' : 'Use a recovery code'}
                </button>
                <button type="button" onClick={handleBackToSignIn} style={linkButtonStyle}>
                  Back to sign in
                </button>
              </div>
            </div>
          </>
        ) : (
          <>
            {/* Tabs */}
            <div style={{ display: 'flex', borderBottom: '1px solid var(--border)', marginBottom: '24px' }}>
              {(['login', 'register'] as const).map(t => (
                <button key={t} onClick={() => { setTab(t); setError('') }} style={{
                  flex: 1, padding: '8px 0', textAlign: 'center',
                  fontSize: '13px', fontWeight: 500, cursor: 'pointer',
                  background: 'transparent', border: 'none',
                  borderBottom: `2px solid ${tab === t ? 'var(--accent)' : 'transparent'}`,
                  marginBottom: '-1px',
                  color: tab === t ? 'var(--accent)' : 'var(--text-mute)',
                  transition: 'color 0.15s, border-color 0.15s',
                }}>
                  {t === 'login' ? 'Sign in' : 'Register'}
                </button>
              ))}
            </div>

            {error && <div style={{ marginBottom: '12px' }}><StatusMessage kind="error">{error}</StatusMessage></div>}

            {/* Fields */}
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              <Input
                label="Username"
                value={username}
                onChange={e => setUsername(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleSubmit()}
                placeholder="your username"
                autoComplete="username"
              />
              <Input
                label="Password"
                type="password"
                value={password}
                onChange={e => setPassword(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleSubmit()}
                placeholder={tab === 'register' ? 'min 8 characters' : '••••••••'}
                autoComplete={tab === 'login' ? 'current-password' : 'new-password'}
              />

              {tab === 'register' && (
                <Input
                  label="Invite Code"
                  value={inviteCode}
                  onChange={e => setInviteCode(e.target.value)}
                  onKeyDown={e => e.key === 'Enter' && handleSubmit()}
                  placeholder="xxxx-xxxx-xxxx"
                />
              )}

              <Button onClick={handleSubmit} disabled={loading} fullWidth style={{ marginTop: '4px' }}>
                {loading ? 'Please wait…' : tab === 'login' ? 'Sign in' : 'Create account'}
              </Button>

              {tab === 'register' && (
                <p style={{ fontSize: '11px', color: 'var(--text-mute)', textAlign: 'center', margin: '4px 0 0', lineHeight: 1.5 }}>
                  By creating an account you agree to our{' '}
                  <a href="/privacy" target="_blank" rel="noopener noreferrer" style={{ color: 'var(--accent)', textDecoration: 'none' }}>
                    Privacy Policy
                  </a>
                </p>
              )}
            </div>
          </>
        )}

        <div style={{ marginTop: '24px', textAlign: 'center', fontSize: '11px', color: 'var(--text-mute)' }}>
          <a href="/privacy" target="_blank" rel="noopener noreferrer" style={{ color: 'var(--text-mute)', textDecoration: 'none' }}>
            Privacy Policy
          </a>
        </div>
      </div>
    </div>
  )
}
