import type { InputHTMLAttributes, ReactNode } from 'react'

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: ReactNode
}

export function Input({ label, style, ...props }: InputProps) {
  return (
    <div>
      {label && (
        <div style={{ fontSize: '12px', fontWeight: 500, color: 'var(--text-dim)', marginBottom: '4px' }}>
          {label}
        </div>
      )}
      <input
        style={{
          width: '100%',
          background: 'var(--bg-3)',
          border: '1px solid var(--border)',
          borderRadius: '8px',
          color: 'var(--text)',
          fontSize: '14px',
          padding: '10px 14px',
          outline: 'none',
          boxSizing: 'border-box',
          fontFamily: 'var(--font)',
          ...style,
        }}
        {...props}
      />
    </div>
  )
}
