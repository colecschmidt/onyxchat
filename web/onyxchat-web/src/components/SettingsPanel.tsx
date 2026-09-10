import { useState } from 'react'
import { useAuth } from '../context/AuthContext'
import { changePassword } from '../api/auth'
import { deleteAccount } from '../api/contacts'
import { Avatar, Button, Input, Modal, ModalHeader, StatusMessage, WarningIcon } from './ui'

type Section = 'account' | 'danger'

interface SettingsPanelProps {
  onClose: () => void
}

export function SettingsPanel({ onClose }: SettingsPanelProps) {
  const { user, logout } = useAuth()
  const [section, setSection] = useState<Section>('account')

  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [pwError, setPwError] = useState('')
  const [pwSuccess, setPwSuccess] = useState(false)
  const [pwLoading, setPwLoading] = useState(false)

  const [deletePassword, setDeletePassword] = useState('')
  const [deleteConfirm, setDeleteConfirm] = useState('')
  const [deleteError, setDeleteError] = useState('')
  const [deleteLoading, setDeleteLoading] = useState(false)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)

  async function handleChangePassword() {
    setPwError('')
    setPwSuccess(false)
    if (!currentPassword || !newPassword || !confirmPassword)
      return setPwError('All fields are required.')
    if (newPassword.length < 8)
      return setPwError('New password must be at least 8 characters.')
    if (newPassword !== confirmPassword)
      return setPwError('New passwords do not match.')

    setPwLoading(true)
    try {
      await changePassword(currentPassword, newPassword)
      setPwSuccess(true)
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
    } catch (e) {
      setPwError(e instanceof Error ? e.message : 'Failed to change password.')
    } finally {
      setPwLoading(false)
    }
  }

  async function handleDeleteAccount() {
    setDeleteError('')
    if (!deletePassword) return setDeleteError('Password is required.')
    if (deleteConfirm !== 'delete my account')
      return setDeleteError('Please type "delete my account" exactly to confirm.')

    setDeleteLoading(true)
    try {
      await deleteAccount(deletePassword)
      await logout()
    } catch (e) {
      setDeleteError(e instanceof Error ? e.message : 'Failed to delete account.')
      setDeleteLoading(false)
    }
  }

  return (
    <Modal onClose={onClose} maxWidth={480}>
      <ModalHeader title="Settings" onClose={onClose} />

      <div style={{ display: 'flex', flex: 1, overflow: 'hidden', margin: '-4px -24px -24px', padding: '4px 24px 24px' }}>
        {/* Sidebar nav */}
        <div style={{
          width: '140px', flexShrink: 0,
          borderRight: '1px solid var(--border)',
          padding: '0 8px 0 0',
          display: 'flex', flexDirection: 'column', gap: '2px',
          marginRight: '12px',
        }}>
          {([
            { id: 'account', label: 'Account' },
            { id: 'danger', label: 'Danger Zone' },
          ] as { id: Section; label: string }[]).map(({ id, label }) => (
            <button key={id} onClick={() => setSection(id)} style={{
              width: '100%', textAlign: 'left',
              padding: '8px 12px', borderRadius: '8px',
              border: 'none', cursor: 'pointer', fontSize: '13px', fontWeight: 500,
              background: section === id ? 'var(--surface)' : 'transparent',
              color: id === 'danger'
                ? section === id ? 'var(--red)' : 'rgba(226,104,92,0.7)'
                : section === id ? 'var(--text)' : 'var(--text-mute)',
              transition: 'background 0.15s, color 0.15s',
              fontFamily: 'var(--font)',
            }}>
              {label}
            </button>
          ))}
        </div>

        {/* Content */}
        <div style={{ flex: 1, overflowY: 'auto' }}>
          {section === 'account' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
              <div style={{
                display: 'flex', alignItems: 'center', gap: '12px',
                padding: '14px 16px',
                background: 'var(--bg-3)', borderRadius: '12px',
                border: '1px solid var(--border)',
              }}>
                <Avatar name={user?.username ?? '?'} size={36} />
                <div>
                  <div style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text)' }}>
                    {user?.username}
                  </div>
                  <div style={{ fontSize: '12px', color: 'var(--text-mute)', fontFamily: 'var(--mono)' }}>
                    ID: {user?.id}
                  </div>
                </div>
              </div>

              <div>
                <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', marginBottom: '14px' }}>
                  Change Password
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                  <Input
                    label="Current Password"
                    type="password" value={currentPassword}
                    onChange={e => setCurrentPassword(e.target.value)}
                    placeholder="••••••••"
                    autoComplete="current-password"
                  />
                  <Input
                    label="New Password"
                    type="password" value={newPassword}
                    onChange={e => setNewPassword(e.target.value)}
                    placeholder="min 8 characters"
                    autoComplete="new-password"
                  />
                  <Input
                    label="Confirm New Password"
                    type="password" value={confirmPassword}
                    onChange={e => setConfirmPassword(e.target.value)}
                    placeholder="••••••••"
                    autoComplete="new-password"
                  />

                  {pwError && <StatusMessage kind="error">{pwError}</StatusMessage>}
                  {pwSuccess && <StatusMessage kind="success">Password updated successfully.</StatusMessage>}

                  <Button onClick={handleChangePassword} disabled={pwLoading} style={{ marginTop: '4px' }}>
                    {pwLoading ? 'Updating…' : 'Update Password'}
                  </Button>
                </div>
              </div>
            </div>
          )}

          {section === 'danger' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              <div style={{
                display: 'flex', gap: '10px',
                padding: '14px 16px',
                background: 'rgba(226,104,92,0.06)',
                border: '1px solid rgba(226,104,92,0.2)',
                borderRadius: '12px',
                fontSize: '13px', color: 'var(--text-dim)', lineHeight: 1.6,
              }}>
                <span style={{ color: 'var(--red)', flexShrink: 0, marginTop: '2px' }}><WarningIcon /></span>
                <span>
                  Deleting your account is <strong style={{ color: 'var(--text)' }}>permanent and irreversible</strong>.
                  All your messages, contacts, and encryption keys will be erased immediately.
                </span>
              </div>

              {!showDeleteConfirm ? (
                <Button variant="danger-outline" onClick={() => setShowDeleteConfirm(true)}>
                  Delete My Account
                </Button>
              ) : (
                <div style={{
                  display: 'flex', flexDirection: 'column', gap: '10px',
                  padding: '16px',
                  background: 'rgba(226,104,92,0.06)',
                  border: '1px solid rgba(226,104,92,0.25)',
                  borderRadius: '12px',
                }}>
                  <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--red)' }}>
                    Confirm account deletion
                  </div>

                  <Input
                    label="Your Password"
                    type="password" value={deletePassword}
                    onChange={e => setDeletePassword(e.target.value)}
                    placeholder="Enter your password"
                    autoComplete="current-password"
                  />

                  <Input
                    label={<>Type <span style={{ color: 'var(--text)', fontFamily: 'var(--mono)', fontSize: '11px' }}>delete my account</span> to confirm</>}
                    value={deleteConfirm}
                    onChange={e => setDeleteConfirm(e.target.value)}
                    placeholder="delete my account"
                  />

                  {deleteError && <StatusMessage kind="error">{deleteError}</StatusMessage>}

                  <div style={{ display: 'flex', gap: '8px', marginTop: '4px' }}>
                    <Button
                      variant="secondary"
                      style={{ flex: 1 }}
                      onClick={() => {
                        setShowDeleteConfirm(false)
                        setDeletePassword('')
                        setDeleteConfirm('')
                        setDeleteError('')
                      }}
                    >
                      Cancel
                    </Button>
                    <Button
                      variant="danger"
                      style={{ flex: 1 }}
                      onClick={handleDeleteAccount}
                      disabled={deleteLoading || deleteConfirm !== 'delete my account'}
                    >
                      {deleteLoading ? 'Deleting…' : 'Delete Account'}
                    </Button>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </Modal>
  )
}
