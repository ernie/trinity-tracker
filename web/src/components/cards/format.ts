// Helpers shared across ServerCard and MatchCard. Exit-reason formatting and
// score-state classification (left winner / right winner / tie / no_contest).

export type ScoreState = 'left' | 'right' | 'tie' | 'no_contest'

const EXIT_REASONS: Record<string, string> = {
  score_limit: 'Score limit reached',
  time_limit: 'Time limit',
  overtime: 'Decided in OT',
  sudden_death: 'Decided in OT',
  intermission: 'Match ended',
  forfeit: 'Forfeit',
  tie: 'Tie',
  no_contest: 'No contest',
  client_disconnect: 'Client disconnect',
}

/** Maps exit_reason enum to human-readable text. Empty string for empty inputs. */
export function formatExitReason(reason?: string | null): string {
  if (!reason) return ''
  if (EXIT_REASONS[reason]) return EXIT_REASONS[reason]
  // Titlecase fallback for unmapped enum values
  return reason.charAt(0).toUpperCase() + reason.slice(1).replace(/_/g, ' ')
}

/** Classifies a two-side score outcome. */
export function classifyScores(left: number, right: number): ScoreState {
  if (left <= 0 && right <= 0) return 'no_contest'
  if (left === right) return 'tie'
  return left > right ? 'left' : 'right'
}

export type MedalType = 'impressive' | 'excellent' | 'humiliation' | 'capture' | 'assist' | 'defend' | 'victory' | 'skull' | 'obelisk'

export interface AwardEntry {
  type: MedalType
  count: number
}

export interface PlayerAwardCounts {
  impressives?: number
  excellents?: number
  humiliations?: number
  captures?: number
  assists?: number
  defends?: number
  victories?: number
  skulls_delivered?: number
  obelisks_destroyed?: number
}

/** Player medal counts → ordered AwardEntry list. Zero/absent dropped.
 *  Order: combat awards (excellence chain) first, then objective awards
 *  (mode-specific scoring/destruction), then assist-class. */
export function awardsFromCounts(p: PlayerAwardCounts): AwardEntry[] {
  const order: Array<[MedalType, number | undefined]> = [
    ['excellent', p.excellents],
    ['impressive', p.impressives],
    ['humiliation', p.humiliations],
    ['victory', p.victories],
    ['capture', p.captures],
    ['skull', p.skulls_delivered],
    ['obelisk', p.obelisks_destroyed],
    ['defend', p.defends],
    ['assist', p.assists],
  ]
  return order
    .filter((entry): entry is [MedalType, number] => typeof entry[1] === 'number' && entry[1] > 0)
    .map(([type, count]) => ({ type, count }))
}
