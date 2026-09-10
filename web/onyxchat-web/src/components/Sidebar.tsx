import { useEffect, useRef, useState } from 'react'
import { useChat } from '../context/ChatContext'
import { useAuth } from '../context/AuthContext'
import { SettingsPanel } from './SettingsPanel'
import { addContact } from '../api/contacts'
import { api } from '../api/client'
import {
  Avatar, Badge, Button, IconButton, Input, Logo, Modal, ModalHeader,
  StatusMessage, PlusIcon, SettingsIcon, LogoutIcon,
} from './ui'

export function Sidebar() {
  const { user, logout } = useAuth()
  const { contacts, activePeer, unread, selectPeer, loadContacts } = useChat()
  const [contactsLoaded, setContactsLoaded] = useState(false)
  const [showSettings, setShowSettings] = useState(false)
  const [showAddContact, setShowAddContact] = useState(false)
  const [addUsername, setAddUsername] = useState('')
  const [addError, setAddError] = useState('')
  const [addLoading, setAddLoading] = useState(false)
  const [addSuccess, setAddSuccess] = useState(false)
  const [searchResults, setSearchResults] = useState<{ id: number; username: string }[]>([])
  const [showDropdown, setShowDropdown] = useState(false)
  const searchTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => { loadContacts().finally(() => setContactsLoaded(true)) }, [loadContacts])

  const online = contacts.filter(c => c.online)
  const offline = contacts.filter(c => !c.online)

  function closeAddContact() {
    setShowAddContact(false)
    setAddUsername('')
    setAddError('')
    setAddSuccess(false)
    setSearchResults([])
    setShowDropdown(false)
  }

  function handleSearchInput(value: string) {
    setAddUsername(value)
    setAddError('')
    if (searchTimer.current) clearTimeout(searchTimer.current)
    if (value.trim().length < 1) {
      setSearchResults([])
      setShowDropdown(false)
      return
    }
    searchTimer.current = setTimeout(async () => {
      try {
        const results = await api.get<{ id: number; username: string }[]>(
          `/api/v1/users?search=${encodeURIComponent(value.trim())}`
        )
        setSearchResults(Array.isArray(results) ? results : [])
        setShowDropdown(true)
      } catch {
        setSearchResults([])
      }
    }, 200)
  }

  function handleSelectResult(username: string) {
    setAddUsername(username)
    setShowDropdown(false)
    setSearchResults([])
  }

  async function handleAddContact() {
    setAddError('')
    setAddSuccess(false)
    if (!addUsername.trim()) return setAddError('Enter a username.')
    setAddLoading(true)
    try {
      await addContact(addUsername.trim())
      await loadContacts()
      setAddSuccess(true)
      setAddUsername('')
      setSearchResults([])
      setTimeout(closeAddContact, 1200)
    } catch (e) {
      setAddError(e instanceof Error ? e.message : 'User not found.')
    } finally {
      setAddLoading(false)
    }
  }

  return (
    <>
      {showAddContact && (
        <Modal onClose={closeAddContact} maxWidth={360}>
          <ModalHeader title="Add Contact" onClose={closeAddContact} />

          <div style={{ position: 'relative' }}>
            <Input
              autoFocus
              value={addUsername}
              onChange={e => handleSearchInput(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter') { setShowDropdown(false); handleAddContact() } if (e.key === 'Escape') setShowDropdown(false) }}
              onBlur={() => setTimeout(() => setShowDropdown(false), 150)}
              placeholder="Search by username…"
            />
            {showDropdown && searchResults.length > 0 && (
              <div style={{
                position: 'absolute', top: 'calc(100% + 4px)', left: 0, right: 0,
                background: 'var(--bg-3)', border: '1px solid var(--border-2)',
                borderRadius: '8px', overflow: 'hidden', zIndex: 10,
                boxShadow: '0 4px 16px rgba(0,0,0,0.4)',
              }}>
                {searchResults.map(u => (
                  <div
                    key={u.id}
                    onMouseDown={() => handleSelectResult(u.username)}
                    style={{
                      padding: '9px 14px', cursor: 'pointer', fontSize: '13px',
                      color: 'var(--text)', display: 'flex', alignItems: 'center', gap: '8px',
                    }}
                    onMouseEnter={e => (e.currentTarget.style.background = 'var(--surface)')}
                    onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
                  >
                    <Avatar name={u.username} size={28} />
                    {u.username}
                  </div>
                ))}
              </div>
            )}
            {showDropdown && searchResults.length === 0 && addUsername.trim().length > 0 && (
              <div style={{
                position: 'absolute', top: 'calc(100% + 4px)', left: 0, right: 0,
                background: 'var(--bg-3)', border: '1px solid var(--border-2)',
                borderRadius: '8px', padding: '10px 14px', zIndex: 10,
                fontSize: '12px', color: 'var(--text-mute)',
              }}>
                No users found
              </div>
            )}
          </div>

          {addError && <StatusMessage kind="error">{addError}</StatusMessage>}
          {addSuccess && <StatusMessage kind="success">Contact added ✓</StatusMessage>}

          <Button onClick={handleAddContact} disabled={addLoading} fullWidth>
            {addLoading ? 'Adding…' : 'Add Contact'}
          </Button>
        </Modal>
      )}

      <div style={{
        width: '260px', minWidth: '260px',
        display: 'flex', flexDirection: 'column',
        background: 'var(--bg-2)',
        borderRight: '1px solid var(--border)',
      }}>
        {/* Header */}
        <div style={{
          height: '56px', display: 'flex', alignItems: 'center',
          padding: '0 16px', borderBottom: '1px solid var(--border)', gap: '10px',
        }}>
          <Logo size={26} />
          <span style={{ fontWeight: 600, fontSize: '15px', color: 'var(--text)' }}>
            Onyx<span style={{ color: 'var(--accent)' }}>Chat</span>
          </span>
        </div>

        {/* Add Contact Button */}
        <div style={{ padding: '8px 10px 0' }}>
          <button onClick={() => setShowAddContact(true)} style={{
            width: '100%', display: 'flex', alignItems: 'center', gap: '8px',
            padding: '8px 12px', borderRadius: '8px',
            background: 'var(--accent-glow)', border: '1px dashed var(--accent-dim)',
            color: 'var(--accent)', fontSize: '13px', fontWeight: 500,
            cursor: 'pointer', fontFamily: 'var(--font)',
          }}>
            <PlusIcon />
            Add Contact
          </button>
        </div>

        {/* Contacts */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '8px 0' }}>
          {online.length > 0 && (
            <>
              <div style={{ fontSize: '10px', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.08em', color: 'var(--text-mute)', padding: '10px 16px 4px' }}>
                Online — {online.length}
              </div>
              {online.map(c => <ContactItem key={c.id} contact={c} active={activePeer?.username === c.username} unread={unread[c.username] ?? 0} onClick={() => selectPeer(c.username)} />)}
            </>
          )}
          {offline.length > 0 && (
            <>
              <div style={{ fontSize: '10px', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.08em', color: 'var(--text-mute)', padding: '10px 16px 4px' }}>
                Contacts
              </div>
              {offline.map(c => <ContactItem key={c.id} contact={c} active={activePeer?.username === c.username} unread={unread[c.username] ?? 0} onClick={() => selectPeer(c.username)} />)}
            </>
          )}
          {contactsLoaded && contacts.length === 0 && (
            <div style={{ padding: '24px', textAlign: 'center', color: 'var(--text-mute)', fontSize: '12px' }}>
              No contacts yet
            </div>
          )}
        </div>

        {/* Footer */}
        <div style={{
          padding: '12px', borderTop: '1px solid var(--border)',
          display: 'flex', alignItems: 'center', gap: '10px',
        }}>
          <Avatar name={user?.username ?? '?'} size={36} />
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontSize: '13px', fontWeight: 500, color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {user?.username}
            </div>
            <div style={{ fontSize: '11px', color: 'var(--green)', display: 'flex', alignItems: 'center', gap: '4px', fontFamily: 'var(--mono)' }}>
              <span style={{ width: '6px', height: '6px', borderRadius: '50%', background: 'var(--green)', display: 'inline-block' }} />
              online
            </div>
          </div>
          <IconButton onClick={() => setShowSettings(true)} title="Settings">
            <SettingsIcon />
          </IconButton>
          <IconButton onClick={logout} title="Sign out">
            <LogoutIcon />
          </IconButton>
        </div>
      </div>

      {showSettings && <SettingsPanel onClose={() => setShowSettings(false)} />}
    </>
  )
}

function ContactItem({ contact, active, unread, onClick }: {
  contact: { username: string; online: boolean }
  active: boolean
  unread: number
  onClick: () => void
}) {
  return (
    <div onClick={onClick} style={{
      display: 'flex', alignItems: 'center', gap: '10px',
      padding: '9px 12px', cursor: 'pointer', borderRadius: '8px',
      margin: '1px 6px', background: active ? 'var(--surface)' : 'transparent',
    }}>
      <Avatar name={contact.username} size={36} online={contact.online} showStatus />
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontSize: '13px', fontWeight: 500, color: active ? 'var(--text)' : 'var(--text-dim)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {contact.username}
        </div>
      </div>
      <Badge count={unread} />
    </div>
  )
}
