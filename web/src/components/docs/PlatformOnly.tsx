import { type ReactNode } from 'react'
import { type Platform } from './platformStorage'
import { usePlatform } from './PlatformContext'

interface PlatformOnlyProps {
  platform: Platform | Platform[]
  children: ReactNode
}

// Transparent platform filter — renders children only when the
// active platform matches, with no chrome of its own. Use when a
// piece of content should adopt the visual style of its surroundings
// (e.g., a troubleshooting <details> in a list of generic ones)
// rather than being called out as a styled aside. Pair with
// PlatformNote when you want the ember-tinted callout treatment.
export function PlatformOnly({ platform, children }: PlatformOnlyProps) {
  const { platform: active } = usePlatform()
  const platforms = Array.isArray(platform) ? platform : [platform]
  if (!platforms.includes(active)) return null
  return <>{children}</>
}
