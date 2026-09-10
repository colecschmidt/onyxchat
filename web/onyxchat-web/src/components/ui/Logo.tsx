// A faceted gem mark — ties the "Onyx" name to the visual identity instead
// of the generic concentric-circles mark most AI-generated logos default to.

export function Logo({ size = 28 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 28 28" fill="none">
      <path
        d="M14 2 L23 9 L19 25 L9 25 L5 9 Z"
        fill="var(--bg-3)"
        stroke="var(--accent)"
        strokeWidth="1.4"
        strokeLinejoin="round"
      />
      <path
        d="M14 2 L14 25 M5 9 L23 9 M9 25 L14 9 L19 25"
        stroke="var(--accent)"
        strokeWidth="0.8"
        strokeOpacity="0.5"
        strokeLinejoin="round"
      />
      <circle cx="17.5" cy="6.5" r="1" fill="var(--accent)" />
    </svg>
  )
}
