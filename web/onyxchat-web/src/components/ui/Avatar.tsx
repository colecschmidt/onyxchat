import type { CSSProperties } from 'react'

interface AvatarProps {
  name: string
  size?: number
  online?: boolean
  showStatus?: boolean
}

export function Avatar({ name, size = 36, online = false, showStatus = false }: AvatarProps) {
  const initials = name.slice(0, 2).toUpperCase()
  const dotSize = Math.max(8, Math.round(size * 0.26))

  const wrapStyle: CSSProperties = {
    position: 'relative',
    width: size,
    height: size,
    flexShrink: 0,
  }

  const circleStyle: CSSProperties = {
    width: size,
    height: size,
    borderRadius: '50%',
    background: 'var(--surface)',
    border: '1px solid var(--border-2)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: Math.round(size * 0.38),
    fontWeight: 600,
    color: 'var(--text-dim)',
  }

  return (
    <div style={wrapStyle}>
      <div style={circleStyle}>{initials}</div>
      {showStatus && (
        <div
          style={{
            position: 'absolute',
            bottom: -1,
            right: -1,
            width: dotSize,
            height: dotSize,
            borderRadius: '50%',
            background: online ? 'var(--green)' : 'var(--text-mute)',
            border: '2px solid var(--bg-2)',
          }}
        />
      )}
    </div>
  )
}
