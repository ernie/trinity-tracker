import type { PlayerStatsResponse } from '../types'

type StatsNumbers = PlayerStatsResponse['stats']

// The 8-cell "Honors" panel rendered on the player profile and the
// player-stats modal. Each entry maps a stats field to the medal asset
// the StatItem cell uses as a rotated background icon.
//
// Ordered for visual rhythm — Victory leads (the laurel medal anchors
// the grid), then the four arena medals, then the four CTF objectives.
export interface HonorEntry {
  label: string
  icon: string
  /** Pull the count from the stats object — keeps the field-name
   *  coupling explicit without leaking field names into the markup. */
  value: (s: StatsNumbers) => number
}

export const HONORS: readonly HonorEntry[] = [
  { label: 'Victory',     icon: '/assets/medals/medal_victory.png',         value: (s) => s.victories },
  { label: 'Excellent',   icon: '/assets/medals/medal_excellent.png',       value: (s) => s.excellents },
  { label: 'Impressive',  icon: '/assets/medals/medal_impressive.png',      value: (s) => s.impressives },
  { label: 'Humiliation', icon: '/assets/medals/medal_gauntlet.png',        value: (s) => s.humiliations },
  { label: 'Captures',    icon: '/assets/medals/medal_capture.png',         value: (s) => s.captures },
  { label: 'Returns',     icon: '/assets/flags/flag_in_base_red.png',       value: (s) => s.flag_returns },
  { label: 'Assists',     icon: '/assets/medals/medal_assist.png',          value: (s) => s.assists },
  { label: 'Defense',     icon: '/assets/medals/medal_defend.png',          value: (s) => s.defends },
] as const
