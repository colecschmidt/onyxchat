import type { ReactNode } from 'react'
import { IconButton } from './IconButton'
import { CloseIcon } from './Icons'

interface ModalProps {
  onClose?: () => void
  maxWidth?: number
  children: ReactNode
}

export function Modal({ onClose, maxWidth = 380, children }: ModalProps) {
  return (
    <div
      onMouseDown={e => { if (e.target === e.currentTarget) onClose?.() }}
      style={{
        position: 'fixed', inset: 0,
        background: 'rgba(8,7,6,0.72)',
        backdropFilter: 'blur(4px)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        zIndex: 100, padding: '20px',
      }}
    >
      <div style={{
        width: '100%', maxWidth,
        background: 'var(--bg-2)',
        border: '1px solid var(--border-2)',
        borderRadius: '18px',
        padding: '24px',
        display: 'flex', flexDirection: 'column', gap: '16px',
        maxHeight: '90vh',
        overflow: 'hidden',
      }}>
        {children}
      </div>
    </div>
  )
}

export function ModalHeader({ title, onClose }: { title: string; onClose: () => void }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
      <span style={{ fontSize: '15px', fontWeight: 600, color: 'var(--text)' }}>{title}</span>
      <IconButton onClick={onClose}>
        <CloseIcon />
      </IconButton>
    </div>
  )
}
