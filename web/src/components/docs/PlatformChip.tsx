import { type Platform } from './platformStorage'
import { PLATFORM_LABELS } from './PlatformContext'

interface PlatformChipProps {
  platform: Platform | Platform[]
}

// Compact platform-specific marker for inline use in headers,
// summary lines, and similar — much smaller visual weight than
// PlatformNote. Use to flag a piece of UI as platform-specific
// without taking over the layout. Pair with PlatformOnly when the
// surrounding content is already filtered (the chip just labels
// what's being shown, it doesn't filter).
export function PlatformChip({ platform }: PlatformChipProps) {
  const platforms = Array.isArray(platform) ? platform : [platform]
  const label = platforms.map((p) => PLATFORM_LABELS[p]).join(' · ')
  return <span className="docs-platform-chip">{label}</span>
}
