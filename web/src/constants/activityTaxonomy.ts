import type { ActivityItem } from '../types'

// Stable keys identifying each user-toggleable activity class. Each
// activity classifies to exactly one. Persisted in localStorage so
// renaming any of these is a breaking change for stored prefs.
export type ActivityFilterKey =
  | 'join'
  | 'leave'
  | 'match_start'
  | 'flag_capture'
  | 'flag_movement'
  | 'obelisk_destroy'
  | 'obelisk_damage'
  | 'skull_score'
  | 'skull_pickup'
  | 'award_impressive'
  | 'award_excellent'
  | 'award_humiliation'
  | 'award_defend'
  | 'award_assist'
  | 'chat'
  | 'team_change'

export interface ActivityFilterEntry {
  key: ActivityFilterKey
  label: string
}

export interface ActivityFilterCategory {
  id: string
  label: string
  entries: ActivityFilterEntry[]
}

// Categorized for the filter panel UI. Order matters for display.
export const ACTIVITY_FILTER_CATEGORIES: ActivityFilterCategory[] = [
  {
    id: 'player',
    label: 'Player activity',
    entries: [
      { key: 'join', label: 'Joins' },
      { key: 'leave', label: 'Leaves' },
    ],
  },
  {
    id: 'gameplay',
    label: 'Gameplay',
    entries: [
      { key: 'flag_capture', label: 'Flag captured' },
      { key: 'flag_movement', label: 'Flag status changed' },
      { key: 'obelisk_destroy', label: 'Obelisk destroyed' },
      { key: 'obelisk_damage', label: 'Obelisk attacked' },
      { key: 'skull_score', label: 'Skulls scored' },
      { key: 'skull_pickup', label: 'Skulls picked up' },
    ],
  },
  {
    id: 'awards',
    label: 'Awards',
    entries: [
      { key: 'award_impressive', label: 'Impressive' },
      { key: 'award_excellent', label: 'Excellent' },
      { key: 'award_humiliation', label: 'Humiliation' },
      { key: 'award_defend', label: 'Defend' },
      { key: 'award_assist', label: 'Assist' },
    ],
  },
  {
    id: 'misc',
    label: 'Misc',
    entries: [
      { key: 'match_start', label: 'Match start' },
      { key: 'chat', label: 'Chat' },
      { key: 'team_change', label: 'Team changes' },
    ],
  },
]

// Keys NOT in this set are off by default. Awards demoted, chat off,
// flag-movement noise off, team changes off, skull score off — per
// the spam-control defaults agreed upon when designing the drawer.
export const ACTIVITY_FILTER_DEFAULT_ON: ReadonlySet<ActivityFilterKey> = new Set([
  'join',
  'leave',
  'match_start',
  'flag_capture',
  'obelisk_destroy',
])

// All keys (useful for select-all and validation).
export const ACTIVITY_FILTER_ALL_KEYS: ActivityFilterKey[] =
  ACTIVITY_FILTER_CATEGORIES.flatMap((c) => c.entries.map((e) => e.key))

// Classify an activity into exactly one filter key. Returns undefined
// for items that don't map to any known class — those bypass filtering
// (always shown) so a new event type doesn't disappear silently.
export function classifyActivity(a: ActivityItem): ActivityFilterKey | undefined {
  if (a.type === 'join') return 'join'
  if (a.type === 'leave') return 'leave'
  if (a.type === 'chat') return 'chat'
  switch (a.activityType) {
    case 'match_start': return 'match_start'
    case 'flag_capture': return 'flag_capture'
    case 'flag_taken':
    case 'flag_drop':
    case 'flag_return':
      return 'flag_movement'
    case 'obelisk_destroy': return 'obelisk_destroy'
    case 'obelisk_damage': return 'obelisk_damage'
    case 'skull_score': return 'skull_score'
    case 'skull_pickup': return 'skull_pickup'
    case 'team_change': return 'team_change'
    case 'award':
      switch (a.awardType) {
        case 'impressive': return 'award_impressive'
        case 'excellent': return 'award_excellent'
        case 'humiliation': return 'award_humiliation'
        case 'defend': return 'award_defend'
        case 'assist': return 'award_assist'
      }
  }
  return undefined
}
