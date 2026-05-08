import type React from 'react'
import type { ScoreState } from './format'

interface ScoreboardProps {
  redLabel: string
  redScore: number
  blueLabel: string
  blueScore: number
  state: ScoreState   // 'left' | 'right' | 'tie' | 'no_contest'
  /** Live games never show TIE / NO CONTEST and never dim a side — the
   *  outcome isn't decided yet, so we always render `vs`. */
  live?: boolean
  /** Optional content rendered next to the red score (CTF flag icon). */
  redIndicator?: React.ReactNode
  /** Optional content rendered next to the blue score (CTF flag icon). */
  blueIndicator?: React.ReactNode
  /** Optional content rendered below `vs` in the center column (1FCTF
   *  neutral flag drift). */
  centerIndicator?: React.ReactNode
}

export function Scoreboard({
  redLabel,
  redScore,
  blueLabel,
  blueScore,
  state,
  live,
  redIndicator,
  blueIndicator,
  centerIndicator,
}: ScoreboardProps) {
  // On a live card the match is in progress: no winner, no loser, no
  // TIE / NO CONTEST badge. Just two scores and `vs`.
  const effectiveState: ScoreState = live ? 'left' /* unused */ : state
  const redClass = !live && effectiveState === 'left' ? 'winner'
                  : !live && effectiveState === 'right' ? 'loser' : ''
  const blueClass = !live && effectiveState === 'right' ? 'winner'
                   : !live && effectiveState === 'left' ? 'loser' : ''

  let connector: React.ReactNode = 'vs'
  if (!live && state === 'tie') connector = <span className="scoreboard__state tie">TIE</span>
  if (!live && state === 'no_contest') connector = <span className="scoreboard__state nc">NO<br />CONTEST</span>

  return (
    <div className="scoreboard">
      <div className="scoreboard__side">
        <span className="scoreboard__label red">{redLabel}</span>
        <span className="scoreboard__score-line">
          <span className={`scoreboard__score ${redClass}`}>{redScore}</span>
          {redIndicator && <span className="scoreboard__indicator">{redIndicator}</span>}
        </span>
      </div>
      <div className="scoreboard__center">
        <span className="scoreboard__vs">{connector}</span>
        {centerIndicator && <span className="scoreboard__center-indicator">{centerIndicator}</span>}
      </div>
      <div className="scoreboard__side">
        <span className="scoreboard__label blue">{blueLabel}</span>
        <span className="scoreboard__score-line">
          {blueIndicator && <span className="scoreboard__indicator">{blueIndicator}</span>}
          <span className={`scoreboard__score ${blueClass}`}>{blueScore}</span>
        </span>
      </div>
    </div>
  )
}
