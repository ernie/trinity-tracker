import type React from 'react'
import { PlayerPortrait } from '../PlayerPortrait'
import { PlayerBadge } from '../PlayerBadge'
import { BotBadge } from '../BotBadge'
import { MedalIcon } from '../MedalIcon'
import { ColoredText } from '../ColoredText'
import type { ScoreState, AwardEntry } from './format'

export interface DuelistData {
  name: string
  cleanName?: string
  /** Q3 model id (e.g. "sarge/default") used by PlayerPortrait. Falls back to initial. */
  model?: string
  isBot?: boolean
  /** 1–5 bot skill, drives BotBadge color. */
  skill?: number
  isVR?: boolean
  isVerified?: boolean
  isAdmin?: boolean
  score: number
  /** sub-text below the score, e.g. "24 ping" or "25 F · 19 D" */
  sub?: React.ReactNode
  awards?: AwardEntry[]
}

interface DuelistsProps {
  left: DuelistData
  right: DuelistData
  state: ScoreState
  /** Live duels: no winner/loser styling, no TIE / NO CONTEST. Just `vs`. */
  live?: boolean
}

function DuelistAwards({ awards, side }: { awards?: AwardEntry[]; side: 'left' | 'right' }) {
  if (!awards || awards.length === 0) {
    return <div className={`duelist-awards ${side === 'right' ? 'right' : ''}`} aria-hidden />
  }
  return (
    <div className={`duelist-awards ${side === 'right' ? 'right' : ''}`}>
      {awards.map((a) => (
        <MedalIcon key={a.type} type={a.type} count={a.count} size="sm" />
      ))}
    </div>
  )
}

function Duelist({ data, side, winnerSide }: { data: DuelistData; side: 'left' | 'right'; winnerSide: 'left' | 'right' | null }) {
  const isWinner = winnerSide === side
  const isLoser = winnerSide !== null && winnerSide !== side
  const portraitFallback = (data.cleanName || data.name || '?').replace(/\^./g, '').charAt(0).toLowerCase() || '?'

  return (
    <div className="duelist">
      <span className={`duelist__portrait-wrap ${isWinner ? 'winner' : ''}`}>
        <PlayerPortrait model={data.model} size="lg" fallback={portraitFallback} />
        {data.isBot ? (
          <BotBadge isBot skill={data.skill ?? 1} size="sm" />
        ) : (
          <PlayerBadge isVerified={data.isVerified} isAdmin={data.isAdmin} isVR={data.isVR} size="sm" />
        )}
      </span>
      <span className={`duelist__name ${isLoser ? 'dim' : ''}`}>
        <ColoredText text={data.name} />
      </span>
      <span className={`duelist__score ${isLoser ? 'loser' : ''} ${isWinner ? 'winner' : ''}`}>{data.score}</span>
      {data.sub && <span className="duelist__sub">{data.sub}</span>}
    </div>
  )
}

export function Duelists({ left, right, state, live }: DuelistsProps) {
  // Live duel: no winner/loser styling and the connector is always `vs`.
  // The match isn't decided, so showing TIE / NO CONTEST is misleading.
  const winnerSide = !live && state === 'left' ? 'left'
                    : !live && state === 'right' ? 'right' : null

  let connector: React.ReactNode = 'vs'
  if (!live && state === 'tie') connector = <span className="scoreboard__state tie">TIE</span>
  if (!live && state === 'no_contest') connector = <span className="scoreboard__state nc">NO<br />CONTEST</span>

  return (
    <div className="duelists">
      <DuelistAwards awards={left.awards} side="left" />
      <Duelist data={left} side="left" winnerSide={winnerSide} />
      <span className="scoreboard__vs">{connector}</span>
      <Duelist data={right} side="right" winnerSide={winnerSide} />
      <DuelistAwards awards={right.awards} side="right" />
    </div>
  )
}
