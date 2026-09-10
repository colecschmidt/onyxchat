import type { ButtonHTMLAttributes, CSSProperties } from 'react'

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'danger-outline'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant
  fullWidth?: boolean
}

const base: CSSProperties = {
  border: 'none',
  borderRadius: '10px',
  fontSize: '13px',
  fontWeight: 600,
  cursor: 'pointer',
  padding: '10px 16px',
  transition: 'opacity 0.15s, background 0.15s',
  fontFamily: 'var(--font)',
}

const variants: Record<Variant, CSSProperties> = {
  primary: { background: 'var(--accent)', color: 'var(--accent-ink)' },
  secondary: { background: 'var(--bg-3)', color: 'var(--text-dim)', border: '1px solid var(--border)' },
  ghost: { background: 'transparent', color: 'var(--text-mute)' },
  danger: { background: 'var(--red)', color: 'white' },
  'danger-outline': { background: 'transparent', color: 'var(--red)', border: '1px solid rgba(226,104,92,0.4)' },
}

export function Button({ variant = 'primary', fullWidth, disabled, style, ...props }: ButtonProps) {
  return (
    <button
      disabled={disabled}
      style={{
        ...base,
        ...variants[variant],
        width: fullWidth ? '100%' : undefined,
        opacity: disabled ? 0.5 : 1,
        cursor: disabled ? 'not-allowed' : 'pointer',
        ...style,
      }}
      {...props}
    />
  )
}
