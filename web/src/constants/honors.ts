import type { PlayerStatsResponse } from '../types'

type StatsNumbers = PlayerStatsResponse['stats']

// "Honors" panel — 10 medal cells. HONORS[0] is the featured honor
// (rendered alongside the portrait in PlayerHero); the remaining 9
// fill a 3x3 grid in HonorsPanel. Order forms three thematic rows:
//   1. Victory (featured) + combat highlights (Excellent/Impressive/Humiliation)
//   2. Scoring captures across game modes (CTF/Harvester/Overload)
//   3. Defensive / support (Returns/Assists/Defense)
export interface HonorEntry {
  label: string
  icon: string
  value: (s: StatsNumbers) => number
}

export const HONORS: readonly HonorEntry[] = [
  { label: 'Victory',     icon: '/assets/medals/medal_victory.png',    value: (s) => s.victories },
  { label: 'Excellent',   icon: '/assets/medals/medal_excellent.png',  value: (s) => s.excellents },
  { label: 'Impressive',  icon: '/assets/medals/medal_impressive.png', value: (s) => s.impressives },
  { label: 'Humiliation', icon: '/assets/medals/medal_gauntlet.png',   value: (s) => s.humiliations },
  { label: 'Captures',    icon: '/assets/medals/medal_capture.png',    value: (s) => s.captures },
  { label: 'Skulls',      icon: '/assets/medals/medal_skull.png',      value: (s) => s.skulls_delivered },
  { label: 'Obelisks',    icon: '/assets/medals/medal_obelisk.png',    value: (s) => s.obelisks_destroyed },
  { label: 'Returns',     icon: '/assets/flags/flag_in_base_red.png',  value: (s) => s.flag_returns },
  { label: 'Assists',     icon: '/assets/medals/medal_assist.png',     value: (s) => s.assists },
  { label: 'Defense',     icon: '/assets/medals/medal_defend.png',     value: (s) => s.defends },
] as const
