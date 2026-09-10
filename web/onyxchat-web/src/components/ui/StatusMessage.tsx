import type { ReactNode } from 'react'

type Kind = 'error' | 'success' | 'warning'

const colors: Record<Kind, { text: string; bg: string; border: string }> = {
  error: { text: 'var(--red)', bg: 'rgba(226,104,92,0.08)', border: 'rgba(226,104,92,0.2)' },
  success: { text: 'var(--green)', bg: 'rgba(127,191,118,0.08)', border: 'rgba(127,191,118,0.2)' },
  warning: { text: 'var(--text-dim)', bg: 'rgba(226,104,92,0.06)', border: 'rgba(226,104,92,0.2)' },
}

export function StatusMessage({ kind, children }: { kind: Kind; children: ReactNode }) {
  const c = colors[kind]
  return (
    <div style={{
      fontSize: '12px', color: c.text, background: c.bg,
      border: `1px solid ${c.border}`, borderRadius: '8px',
      padding: '8px 12px', lineHeight: 1.5,
    }}>
      {children}
    </div>
  )
}
