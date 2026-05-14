import type { ReactNode } from 'react'
import { FlagPair } from '../components/FlagPair'
import type { PlayerStatsResponse } from '../types'

type StatsNumbers = PlayerStatsResponse['stats']

// "Honors" panel — 10 medal cells. The HONORS entry whose `key` matches
// `player.featured_honor` is rendered alongside the portrait in
// PlayerHero; the remaining 9 fill a 3x3 grid in HonorsPanel. Default
// featured key is 'victories' (HONORS[0]). Order forms three thematic rows:
//   1. Victory (featured by default) + combat highlights (Excellent/Impressive/Humiliation)
//   2. Scoring captures across game modes (CTF/Harvester/Overload)
//   3. Defensive / support (Returns/Assists/Defense)
//
// `key` is the stable identifier persisted on the server (users.featured_honor)
// and validated against the allow-list in internal/api/featured_honor.go. Keep
// the two lists in sync — any new entry here needs its key added there too.
export interface HonorEntry {
  key: string
  label: string
  /** URL used as the simple <img> source. Always set so we have a sane
   *  default for share-cards / future contexts; `iconElement` overrides
   *  it in surfaces that want the richer composed treatment. */
  icon: string
  /** Optional composed-emblem override (e.g. crossed-flag pair for
   *  flag_returns). When present, HonorsPanel and PlayerHero render
   *  this element in place of an <img src={icon}>. */
  iconElement?: ReactNode
  value: (s: StatsNumbers) => number
}

export const HONORS: readonly HonorEntry[] = [
  { key: 'victories',          label: 'Victory',     icon: '/assets/medals/medal_victory.png',    value: (s) => s.victories },
  { key: 'excellents',         label: 'Excellent',   icon: '/assets/medals/medal_excellent.png',  value: (s) => s.excellents },
  { key: 'impressives',        label: 'Impressive',  icon: '/assets/medals/medal_impressive.png', value: (s) => s.impressives },
  { key: 'humiliations',       label: 'Humiliation', icon: '/assets/medals/medal_gauntlet.png',   value: (s) => s.humiliations },
  { key: 'captures',           label: 'Captures',    icon: '/assets/medals/medal_capture.png',    value: (s) => s.captures },
  { key: 'skulls_delivered',   label: 'Skulls',      icon: '/assets/medals/medal_skull.png',      value: (s) => s.skulls_delivered },
  { key: 'obelisks_destroyed', label: 'Obelisks',    icon: '/assets/medals/medal_obelisk.png',    value: (s) => s.obelisks_destroyed },
  { key: 'flag_returns',       label: 'Returns',     icon: '/assets/flags/flag_in_base_red.png',  iconElement: <FlagPair />, value: (s) => s.flag_returns },
  { key: 'assists',            label: 'Assists',     icon: '/assets/medals/medal_assist.png',     value: (s) => s.assists },
  { key: 'defends',            label: 'Defense',     icon: '/assets/medals/medal_defend.png',     value: (s) => s.defends },
] as const

/** Default featured honor when a player's owning user hasn't picked one
 *  (or when there's no owning user). Mirrors HONORS[0] but referenced by
 *  key for clarity at call sites. */
export const DEFAULT_FEATURED_HONOR_KEY = 'victories'
