import type React from 'react'
import type { ScoreState } from './format'

export interface DuelistData {
  name: string
  /** initial letter (or short avatar ref) for the placeholder portrait */
  portraitChar: string
  score: number
  /** sub-text below the score, e.g. "24 ping" or "25 F · 19 D" */
  sub?: React.ReactNode
  /** Q3 medals — short codes like 'excellent', 'impressive', 'humiliation', 'gauntlet' */
  awards?: string[]
}

interface DuelistsProps {
  left: DuelistData
  right: DuelistData
  state: ScoreState
}

function MedalIconStub({ kind }: { kind: string }) {
  // Mockup placeholder — production callers can replace with <MedalIcon> when wired up.
  return <span className={`medal medal-${kind}`}>★</span>
}

export function Duelists({ left, right, state }: DuelistsProps) {
  const leftCls = state === 'left' ? 'winner' : state === 'right' ? 'loser' : ''
  const rightCls = state === 'right' ? 'winner' : state === 'left' ? 'loser' : ''
  const leftScoreCls = state === 'right' ? 'loser' : state === 'no_contest' ? 'loser' : ''
  const rightScoreCls = state === 'left' ? 'loser' : state === 'no_contest' ? 'loser' : ''

  let connector: React.ReactNode = 'vs'
  if (state === 'tie') connector = <span className="scoreboard__state tie">TIE</span>
  if (state === 'no_contest') connector = <span className="scoreboard__state nc">NO<br />CONTEST</span>

  return (
    <div className="duelists">
      <div className="duelist-awards">
        {(left.awards ?? []).map((m, i) => <MedalIconStub key={i} kind={m} />)}
      </div>
      <div className="duelist">
        <div className={`duelist__portrait ${leftCls}`}>{left.portraitChar}</div>
        <span className={`duelist__name ${leftCls === 'loser' ? 'dim' : ''}`}>{left.name}</span>
        <span className={`duelist__score ${leftScoreCls}`}>{left.score}</span>
        {left.sub && <span className="duelist__sub">{left.sub}</span>}
      </div>
      <span className="scoreboard__vs">{connector}</span>
      <div className="duelist">
        <div className={`duelist__portrait ${rightCls}`}>{right.portraitChar}</div>
        <span className={`duelist__name ${rightCls === 'loser' ? 'dim' : ''}`}>{right.name}</span>
        <span className={`duelist__score ${rightScoreCls}`}>{right.score}</span>
        {right.sub && <span className="duelist__sub">{right.sub}</span>}
      </div>
      <div className="duelist-awards right">
        {(right.awards ?? []).map((m, i) => <MedalIconStub key={i} kind={m} />)}
      </div>
    </div>
  )
}
