export function Badge({ count }: { count: number }) {
  if (count <= 0) return null
  return (
    <div style={{
      background: 'var(--accent)', color: 'var(--accent-ink)', fontSize: '10px',
      fontWeight: 700, padding: '2px 6px', borderRadius: '999px',
      minWidth: '18px', textAlign: 'center', fontFamily: 'var(--mono)',
    }}>
      {count}
    </div>
  )
}
