import { useEffect } from 'react'
import { useChat } from '../context/ChatContext'
import { useAuth } from '../context/AuthContext'

const initials = (name: string) => name.slice(0, 2).toUpperCase()

export function Sidebar() {
  const { user, logout } = useAuth()
  const { contacts, activePeer, unread, selectPeer, loadContacts } = useChat()

  useEffect(() => { loadContacts() }, [loadContacts])

  const online  = contacts.filter(c => c.online)
  const offline = contacts.filter(c => !c.online)

  return (
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
        <svg width="28" height="28" viewBox="0 0 28 28" fill="none">
          <circle cx="14" cy="14" r="8" stroke="#2563eb" strokeWidth="1.8"/>
          <circle cx="14" cy="14" r="3.5" fill="#2563eb"/>
          <circle cx="14" cy="6" r="1.5" fill="#2563eb" opacity="0.4"/>
        </svg>
        <span style={{ fontWeight: 600, fontSize: '15px', color: 'var(--text)' }}>OnyxChat</span>
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
        {contacts.length === 0 && (
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
        <div style={{
          width: '36px', height: '36px', borderRadius: '50%',
          background: 'var(--surface)', border: '1px solid var(--border-2)',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          fontSize: '14px', fontWeight: 600, color: 'var(--text-dim)', flexShrink: 0,
        }}>
          {user ? initials(user.username) : '?'}
        </div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontSize: '13px', fontWeight: 500, color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {user?.username}
          </div>
          <div style={{ fontSize: '11px', color: 'var(--green)', display: 'flex', alignItems: 'center', gap: '4px' }}>
            <span style={{ width: '6px', height: '6px', borderRadius: '50%', background: 'var(--green)', display: 'inline-block' }} />
            online
          </div>
        </div>
        <button onClick={logout} title="Sign out" style={{
          background: 'transparent', border: 'none', color: 'var(--text-mute)',
          cursor: 'pointer', padding: '6px', borderRadius: '8px', display: 'flex',
        }}>
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>
            <polyline points="16 17 21 12 16 7"/>
            <line x1="21" y1="12" x2="9" y2="12"/>
          </svg>
        </button>
      </div>
    </div>
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
      <div style={{
        width: '36px', height: '36px', borderRadius: '50%',
        background: 'var(--surface)', border: '1px solid var(--border-2)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        fontSize: '14px', fontWeight: 600, color: 'var(--text-dim)',
        flexShrink: 0, position: 'relative',
      }}>
        {initials(contact.username)}
        <div style={{
          position: 'absolute', bottom: '1px', right: '1px',
          width: '9px', height: '9px', borderRadius: '50%',
          background: contact.online ? 'var(--green)' : 'var(--text-mute)',
          border: '2px solid var(--bg-2)',
        }} />
      </div>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontSize: '13px', fontWeight: 500, color: active ? 'var(--text)' : 'var(--text-dim)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {contact.username}
        </div>
      </div>
      {unread > 0 && (
        <div style={{
          background: 'var(--blue)', color: 'white', fontSize: '10px',
          fontWeight: 600, padding: '2px 6px', borderRadius: '999px', minWidth: '18px', textAlign: 'center',
        }}>
          {unread}
        </div>
      )}
    </div>
  )
}