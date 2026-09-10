import type { ButtonHTMLAttributes } from 'react'

export function IconButton({ style, ...props }: ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      style={{
        background: 'transparent',
        border: 'none',
        color: 'var(--text-mute)',
        cursor: 'pointer',
        padding: '6px',
        borderRadius: '8px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        transition: 'background 0.15s, color 0.15s',
        ...style,
      }}
      {...props}
    />
  )
}
