import type { PlayerStatsResponse } from '../types'

type StatsNumbers = PlayerStatsResponse['stats']

// "Honors" panel — 10 medal cells on the player profile + stats modal.
// Each entry maps a stats field to the medal asset used as a rotated
// background icon. Order = visual rhythm: Victory, combat, objectives.
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
  { label: 'Returns',     icon: '/assets/flags/flag_in_base_red.png',      value: (s) => s.flag_returns },
  { label: 'Assists',     icon: '/assets/medals/medal_assist.png',         value: (s) => s.assists },
  { label: 'Defense',     icon: '/assets/medals/medal_defend.png',         value: (s) => s.defends },
  { label: 'Skulls',      icon: '/assets/medals/medal_skull.png',          value: (s) => s.skulls_delivered },
  { label: 'Obelisks',    icon: '/assets/medals/medal_obelisk.png',        value: (s) => s.obelisks_destroyed },
] as const
