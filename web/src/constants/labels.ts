import type { TimePeriod } from '../types'

export const PERIOD_LABELS: Record<TimePeriod, string> = {
  all: 'All Time',
  day: '24 Hours',
  week: '7 Days',
  month: '30 Days',
  year: 'Past Year',
}

export type GameTypeFilter = 'all' | 'ffa' | 'tdm' | 'ctf' | '1fctf' | '1v1' | 'overload' | 'harvester'

export const GAME_TYPE_LABELS: Record<GameTypeFilter, string> = {
  all: 'All',
  ffa: 'FFA',
  '1v1': '1v1',
  tdm: 'TDM',
  ctf: 'CTF',
  '1fctf': '1F CTF',
  overload: 'Overload',
  harvester: 'Harvester',
}
