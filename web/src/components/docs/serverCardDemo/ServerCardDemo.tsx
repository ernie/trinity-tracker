// ServerCardDemo — renders a ServerCard against a static snapshot,
// wrapped in HelpModeRoot so the card's data-help attributes light up
// as hover/tap tooltips. Used inside the "Reading a server card"
// section of /docs/play, one instance per mode.
//
// attackersOverride is only used by the Overload subsection to demo
// the under-attack indicator + per-row reticle without a live data feed.
import type { ServerStatus } from '../../../types'
import { HelpModeRoot } from '../../HelpModeRoot'
import { ServerCard } from '../../ServerCard'

interface ServerCardDemoProps {
  snapshot: ServerStatus
  attackersOverride?: Set<number>
}

export function ServerCardDemo({ snapshot, attackersOverride }: ServerCardDemoProps) {
  return (
    <HelpModeRoot>
      <ServerCard
        server={snapshot}
        presentation="docs"
        attackersOverride={attackersOverride}
      />
    </HelpModeRoot>
  )
}
