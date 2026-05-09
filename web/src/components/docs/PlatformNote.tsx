import { type ReactNode } from 'react'
import { type Platform } from './platformStorage'
import { PLATFORM_LABELS } from './PlatformContext'

interface PlatformNoteProps {
  platform: Platform | Platform[]
  children: ReactNode
}

// Inline callout for a small platform-specific aside. Always
// rendered; the platform chip in the header tells the reader which
// audience it's for. Use when the deviation is small enough that a
// full PlatformTabs block would be overkill.
export function PlatformNote({ platform, children }: PlatformNoteProps) {
  const platforms = Array.isArray(platform) ? platform : [platform]
  const label = platforms.map((p) => PLATFORM_LABELS[p]).join(' · ')

  return (
    <aside className="docs-platform-note">
      <div className="docs-platform-note__header">
        <span className="docs-platform-note__chip">{label}</span>
      </div>
      <div className="docs-platform-note__body">{children}</div>
    </aside>
  )
}
