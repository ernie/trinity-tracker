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
