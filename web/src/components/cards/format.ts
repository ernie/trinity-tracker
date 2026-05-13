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
 *  Order is scoring-relevance ascending: combat-skill medals first
 *  (least directly tied to score), then support, then mode-specific
 *  objective medals, with `victory` last as the match-outcome capstone.
 *  In row-based layouts the strip sits left of the score column, so
 *  the LAST medal renders adjacent to the score — the medal closest
 *  to the score is the one most directly responsible for it. */
export function awardsFromCounts(p: PlayerAwardCounts): AwardEntry[] {
  const order: Array<[MedalType, number | undefined]> = [
    // Combat-skill medals (least direct contribution to score)
    ['humiliation', p.humiliations],
    ['impressive', p.impressives],
    ['excellent', p.excellents],
    // Support (helping teammates score)
    ['assist', p.assists],
    ['defend', p.defends],
    // Mode-specific objective points (direct score contribution)
    ['capture', p.captures],
    ['skull', p.skulls_delivered],
    ['obelisk', p.obelisks_destroyed],
    // Outcome (the match win itself)
    ['victory', p.victories],
  ]
  return order
    .filter((entry): entry is [MedalType, number] => typeof entry[1] === 'number' && entry[1] > 0)
    .map(([type, count]) => ({ type, count }))
}
