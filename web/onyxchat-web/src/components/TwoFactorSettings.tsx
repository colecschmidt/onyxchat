import { useCallback, useEffect, useState } from 'react'
import {
  getTwoFactorStatus,
  startTwoFactorSetup,
  confirmTwoFactorSetup,
  disableTwoFactor,
  regenerateRecoveryCodes,
} from '../api/twofactor'
import { Button, Input, StatusMessage, WarningIcon } from './ui'
import type { TwoFactorStatus, TwoFactorSetup } from '../types'

// Enrollment is deliberately a two-step commit — see api/twofactor.ts. The
// stages below mirror that: 'password' and 'scan' both precede the factor
// actually turning on, so backing out of either leaves the account untouched.
type Stage =
  | 'loading'
  | 'idle'
  | 'password'   // re-auth before a new secret is issued
  | 'scan'       // QR shown, waiting for proof the app holds the secret
  | 'codes'      // recovery codes displayed once
  | 'disable'
  | 'regenerate'

const LOW_RECOVERY_CODES = 3

export function TwoFactorSettings({ username }: { username: string }) {
  const [stage, setStage] = useState<Stage>('loading')
  const [status, setStatus] = useState<TwoFactorStatus | null>(null)
  const [setup, setSetup] = useState<TwoFactorSetup | null>(null)
  const [codes, setCodes] = useState<string[]>([])

  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const [copied, setCopied] = useState(false)

  const refreshStatus = useCallback(async () => {
    const s = await getTwoFactorStatus()
    setStatus(s)
    return s
  }, [])

  useEffect(() => {
    let mounted = true
    refreshStatus()
      .then(() => { if (mounted) setStage('idle') })
      .catch(e => {
        if (!mounted) return
        setError(e instanceof Error ? e.message : 'Could not load two-factor status.')
        setStage('idle')
      })
    return () => { mounted = false }
  }, [refreshStatus])

  function resetInputs() {
    setPassword('')
    setCode('')
    setError('')
  }

  function backToIdle() {
    resetInputs()
    setSetup(null)
    setCodes([])
    setCopied(false)
    setStage('idle')
  }

  // run wraps every mutating action: one place for the busy flag and for
  // turning a rejected promise into a message the user can act on.
  async function run(fn: () => Promise<void>) {
    setError('')
    setNotice('')
    setBusy(true)
    try {
      await fn()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }

  const handleStartSetup = () => run(async () => {
    if (!password) throw new Error('Enter your password to continue.')
    const data = await startTwoFactorSetup(password)
    setSetup(data)
    setPassword('')
    setCode('')
    setStage('scan')
  })

  const handleConfirm = () => run(async () => {
    if (!code.trim()) throw new Error('Enter the 6-digit code from your authenticator app.')
    const { recovery_codes } = await confirmTwoFactorSetup(code)
    setCodes(recovery_codes)
    setSetup(null)
    resetInputs()
    await refreshStatus()
    setStage('codes')
  })

  const handleDisable = () => run(async () => {
    if (!password) throw new Error('Enter your password to continue.')
    if (!code.trim()) throw new Error('Enter a code from your authenticator app, or a recovery code.')
    await disableTwoFactor(password, code)
    resetInputs()
    await refreshStatus()
    setStage('idle')
    setNotice('Two-factor authentication is off.')
  })

  const handleRegenerate = () => run(async () => {
    if (!password) throw new Error('Enter your password to continue.')
    if (!code.trim()) throw new Error('Enter a code from your authenticator app, or a recovery code.')
    const { recovery_codes } = await regenerateRecoveryCodes(password, code)
    setCodes(recovery_codes)
    resetInputs()
    await refreshStatus()
    setStage('codes')
  })

  async function copyCodes() {
    try {
      await navigator.clipboard.writeText(codes.join('\n'))
      setCopied(true)
    } catch {
      setError('Could not copy to the clipboard — select the codes and copy them manually.')
    }
  }

  function downloadCodes() {
    const body = [
      'OnyxChat recovery codes',
      `Account: ${username}`,
      `Generated: ${new Date().toISOString()}`,
      '',
      'Each code can be used once, in place of your authenticator app.',
      'Keep them somewhere safe and offline.',
      '',
      ...codes,
      '',
    ].join('\n')

    const url = URL.createObjectURL(new Blob([body], { type: 'text/plain' }))
    const a = document.createElement('a')
    a.href = url
    a.download = `onyxchat-recovery-codes-${username}.txt`
    a.click()
    URL.revokeObjectURL(url)
  }

  const heading = (text: string) => (
    <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', marginBottom: '10px' }}>
      {text}
    </div>
  )

  const hint = (text: string) => (
    <p style={{ fontSize: '12px', color: 'var(--text-mute)', margin: '0 0 12px', lineHeight: 1.6 }}>
      {text}
    </p>
  )

  if (stage === 'loading') {
    return <div style={{ fontSize: '13px', color: 'var(--text-mute)' }}>Loading…</div>
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
      {stage === 'idle' && (
        <>
          <div style={{
            display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '12px',
            padding: '14px 16px',
            background: 'var(--bg-3)', borderRadius: '12px',
            border: '1px solid var(--border)',
          }}>
            <div>
              <div style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text)' }}>
                Authenticator app
              </div>
              <div style={{ fontSize: '12px', color: 'var(--text-mute)', marginTop: '2px' }}>
                {status?.enabled
                  ? `On · ${status.recovery_codes_remaining} recovery ${status.recovery_codes_remaining === 1 ? 'code' : 'codes'} left`
                  : 'Off — your password is the only thing protecting this account'}
              </div>
            </div>
            <div style={{
              fontSize: '11px', fontWeight: 600, padding: '4px 10px', borderRadius: '999px',
              color: status?.enabled ? 'var(--green)' : 'var(--text-mute)',
              background: status?.enabled ? 'rgba(127,191,118,0.1)' : 'var(--surface)',
              border: `1px solid ${status?.enabled ? 'rgba(127,191,118,0.25)' : 'var(--border)'}`,
              flexShrink: 0,
            }}>
              {status?.enabled ? 'ON' : 'OFF'}
            </div>
          </div>

          {error && <StatusMessage kind="error">{error}</StatusMessage>}
          {notice && <StatusMessage kind="success">{notice}</StatusMessage>}

          {status?.enabled && status.recovery_codes_remaining <= LOW_RECOVERY_CODES && (
            <StatusMessage kind="warning">
              <span style={{ display: 'flex', gap: '8px' }}>
                <span style={{ color: 'var(--red)', flexShrink: 0 }}><WarningIcon /></span>
                <span>
                  You have {status.recovery_codes_remaining} recovery{' '}
                  {status.recovery_codes_remaining === 1 ? 'code' : 'codes'} left. Generate a new set
                  so you don't lose access if your authenticator app is unavailable.
                </span>
              </span>
            </StatusMessage>
          )}

          {status?.enabled ? (
            <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
              <Button variant="secondary" onClick={() => { resetInputs(); setNotice(''); setStage('regenerate') }}>
                New recovery codes
              </Button>
              <Button variant="danger-outline" onClick={() => { resetInputs(); setNotice(''); setStage('disable') }}>
                Turn off
              </Button>
            </div>
          ) : (
            <div>
              {status?.pending_setup && hint(
                'You started setting this up but never confirmed it. Starting again issues a new code.',
              )}
              <Button onClick={() => { resetInputs(); setNotice(''); setStage('password') }}>
                Set up authenticator app
              </Button>
            </div>
          )}
        </>
      )}

      {stage === 'password' && (
        <div>
          {heading('Confirm your password')}
          {hint('Turning on two-factor authentication changes how you sign in, so we ask for your password first.')}
          <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
            <Input
              label="Password"
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && handleStartSetup()}
              placeholder="••••••••"
              autoComplete="current-password"
              autoFocus
            />
            {error && <StatusMessage kind="error">{error}</StatusMessage>}
            <div style={{ display: 'flex', gap: '8px', marginTop: '4px' }}>
              <Button variant="secondary" style={{ flex: 1 }} onClick={backToIdle}>Cancel</Button>
              <Button style={{ flex: 1 }} onClick={handleStartSetup} disabled={busy}>
                {busy ? 'Please wait…' : 'Continue'}
              </Button>
            </div>
          </div>
        </div>
      )}

      {stage === 'scan' && setup && (
        <div>
          {heading('Scan this with your authenticator app')}
          {hint('Use 1Password, Aegis, Google Authenticator, or any other TOTP app. Then enter the 6-digit code it shows to finish.')}

          <div style={{
            display: 'flex', justifyContent: 'center',
            padding: '16px', marginBottom: '12px',
            background: '#fff', borderRadius: '12px', border: '1px solid var(--border)',
          }}>
            <img src={setup.qr_code} alt="Two-factor setup QR code" width={200} height={200} />
          </div>

          <div style={{ marginBottom: '12px' }}>
            <div style={{ fontSize: '12px', color: 'var(--text-mute)', marginBottom: '4px' }}>
              Can't scan? Enter this key manually:
            </div>
            <div style={{
              fontFamily: 'var(--mono)', fontSize: '12px', color: 'var(--text)',
              background: 'var(--bg-3)', border: '1px solid var(--border)',
              borderRadius: '8px', padding: '10px 12px', wordBreak: 'break-all',
              userSelect: 'all',
            }}>
              {setup.secret}
            </div>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
            <Input
              label="Verification code"
              value={code}
              onChange={e => setCode(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && handleConfirm()}
              placeholder="123456"
              autoComplete="one-time-code"
              style={{ fontFamily: 'var(--mono)', letterSpacing: '0.08em' }}
            />
            {error && <StatusMessage kind="error">{error}</StatusMessage>}
            <div style={{ display: 'flex', gap: '8px', marginTop: '4px' }}>
              <Button variant="secondary" style={{ flex: 1 }} onClick={backToIdle}>Cancel</Button>
              <Button style={{ flex: 1 }} onClick={handleConfirm} disabled={busy}>
                {busy ? 'Verifying…' : 'Verify and enable'}
              </Button>
            </div>
          </div>
        </div>
      )}

      {stage === 'codes' && (
        <div>
          {heading('Save your recovery codes')}
          <StatusMessage kind="warning">
            <span style={{ display: 'flex', gap: '8px' }}>
              <span style={{ color: 'var(--red)', flexShrink: 0 }}><WarningIcon /></span>
              <span>
                This is the only time these are shown. Each one works once, in place of your
                authenticator app — without them, losing your device means losing the account.
              </span>
            </span>
          </StatusMessage>

          <div style={{
            display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: '6px',
            margin: '12px 0',
            fontFamily: 'var(--mono)', fontSize: '12px', color: 'var(--text)',
            background: 'var(--bg-3)', border: '1px solid var(--border)',
            borderRadius: '10px', padding: '14px',
            userSelect: 'all',
          }}>
            {codes.map(c => <div key={c}>{c}</div>)}
          </div>

          {error && <StatusMessage kind="error">{error}</StatusMessage>}

          <div style={{ display: 'flex', gap: '8px', marginTop: '10px', flexWrap: 'wrap' }}>
            <Button variant="secondary" onClick={copyCodes}>{copied ? 'Copied' : 'Copy'}</Button>
            <Button variant="secondary" onClick={downloadCodes}>Download</Button>
            <Button style={{ marginLeft: 'auto' }} onClick={() => { backToIdle(); setNotice('Two-factor authentication is on.') }}>
              I've saved them
            </Button>
          </div>
        </div>
      )}

      {(stage === 'disable' || stage === 'regenerate') && (
        <div>
          {heading(stage === 'disable' ? 'Turn off two-factor authentication' : 'Generate new recovery codes')}
          {hint(stage === 'disable'
            ? 'Your account will be protected by your password alone. Confirm with your password and a current code.'
            : 'Your existing recovery codes stop working as soon as the new ones are issued.')}

          <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
            <Input
              label="Password"
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              placeholder="••••••••"
              autoComplete="current-password"
              autoFocus
            />
            <Input
              label="Authenticator or recovery code"
              value={code}
              onChange={e => setCode(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && (stage === 'disable' ? handleDisable() : handleRegenerate())}
              placeholder="123456"
              autoComplete="one-time-code"
              style={{ fontFamily: 'var(--mono)', letterSpacing: '0.08em' }}
            />
            {error && <StatusMessage kind="error">{error}</StatusMessage>}
            <div style={{ display: 'flex', gap: '8px', marginTop: '4px' }}>
              <Button variant="secondary" style={{ flex: 1 }} onClick={backToIdle}>Cancel</Button>
              {stage === 'disable' ? (
                <Button variant="danger" style={{ flex: 1 }} onClick={handleDisable} disabled={busy}>
                  {busy ? 'Turning off…' : 'Turn off'}
                </Button>
              ) : (
                <Button style={{ flex: 1 }} onClick={handleRegenerate} disabled={busy}>
                  {busy ? 'Generating…' : 'Generate'}
                </Button>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
