import type React from 'react'
import type { ScoreState } from './format'

interface ScoreboardProps {
  redLabel: string
  redScore: number
  blueLabel: string
  blueScore: number
  state: ScoreState   // 'left' | 'right' | 'tie' | 'no_contest'
}

export function Scoreboard({ redLabel, redScore, blueLabel, blueScore, state }: ScoreboardProps) {
  const redClass = state === 'left' ? 'winner' : state === 'right' ? 'loser' : ''
  const blueClass = state === 'right' ? 'winner' : state === 'left' ? 'loser' : ''

  let connector: React.ReactNode = 'vs'
  if (state === 'tie') connector = <span className="scoreboard__state tie">TIE</span>
  if (state === 'no_contest') connector = <span className="scoreboard__state nc">NO<br />CONTEST</span>

  return (
    <div className="scoreboard">
      <div className="scoreboard__side">
        <span className="scoreboard__label red">{redLabel}</span>
        <span className={`scoreboard__score ${redClass}`}>{redScore}</span>
      </div>
      <span className="scoreboard__vs">{connector}</span>
      <div className="scoreboard__side">
        <span className="scoreboard__label blue">{blueLabel}</span>
        <span className={`scoreboard__score ${blueClass}`}>{blueScore}</span>
      </div>
    </div>
  )
}
