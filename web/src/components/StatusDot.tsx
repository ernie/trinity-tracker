// Small status circle used wherever the prior ●/○ glyphs were inline.
// `active` toggles between filled (live/running) and hollow (idle).
// currentColor controls hue so callers can theme via CSS class.
export function StatusDot({
  size = 8,
  active = false,
  className,
}: {
  size?: number
  active?: boolean
  className?: string
}) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 8 8"
      fill={active ? 'currentColor' : 'none'}
      stroke="currentColor"
      strokeWidth="1.2"
      className={className}
      aria-hidden="true"
    >
      <circle cx="4" cy="4" r="3" />
    </svg>
  )
}
