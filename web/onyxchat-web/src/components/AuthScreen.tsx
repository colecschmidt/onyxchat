import { useState } from 'react'
import { useAuth } from '../context/AuthContext'
import { Logo, Input, Button, StatusMessage } from './ui'

export function AuthScreen() {
  const { login, register } = useAuth()
  const [tab, setTab] = useState<'login' | 'register'>('login')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [inviteCode, setInviteCode] = useState('')

  async function handleSubmit() {
    setError('')
    if (!username || !password) return setError('Username and password required.')
    if (tab === 'register' && password.length < 8) return setError('Password must be at least 8 characters.')

    setLoading(true)
    try {
      if (tab === 'login') await login(username, password)
      else await register(username, password, inviteCode)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setLoading(false)
    }
  }

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

        <div style={{ marginTop: '24px', textAlign: 'center', fontSize: '11px', color: 'var(--text-mute)' }}>
          <a href="/privacy" target="_blank" rel="noopener noreferrer" style={{ color: 'var(--text-mute)', textDecoration: 'none' }}>
            Privacy Policy
          </a>
        </div>
      </div>
    </div>
  )
}
