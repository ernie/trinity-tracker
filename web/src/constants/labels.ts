import type { TimePeriod } from '../types'

export const PERIOD_LABELS: Record<TimePeriod, string> = {
  all: 'All Time',
  day: '24 Hours',
  week: '7 Days',
  month: '30 Days',
  year: 'Past Year',
}

export type GameTypeFilter = 'all' | 'ffa' | 'tdm' | 'ctf' | '1fctf' | '1v1' | 'overload' | 'harvester'

// Filter-button order; 'all' is rendered separately. Labels via formatGameType().
export const GAME_TYPES: readonly Exclude<GameTypeFilter, 'all'>[] = [
  '1fctf', '1v1', 'ctf', 'ffa', 'harvester', 'overload', 'tdm',
]

export function isGameTypeFilter(s: string): s is GameTypeFilter {
  return s === 'all' || (GAME_TYPES as readonly string[]).includes(s)
}
