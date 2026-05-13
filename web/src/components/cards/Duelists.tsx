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
  /** sub-text below the score, e.g. "24 ping" or "25 K · 19 D" */
  sub?: React.ReactNode
  awards?: AwardEntry[]
  /** DB player ID — opens PlayerStatsModal on click. */
  playerId?: number
}

interface DuelistsProps {
  left: DuelistData
  right: DuelistData
  state: ScoreState
  /** Live: always `vs`, no winner/loser styling. */
  live?: boolean
  onPlayerClick?: (playerName: string, cleanName: string, playerId?: number) => void
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

interface DuelistProps {
  data: DuelistData
  side: 'left' | 'right'
  winnerSide: 'left' | 'right' | null
  onClick?: (playerName: string, cleanName: string, playerId?: number) => void
}

function Duelist({ data, side, winnerSide, onClick }: DuelistProps) {
  const isWinner = winnerSide === side
  const isLoser = winnerSide !== null && winnerSide !== side
  const portraitFallback = (data.cleanName || data.name || '?').replace(/\^./g, '').charAt(0).toLowerCase() || '?'
  const clickable = !!onClick && !data.isBot
  const handleClick = clickable
    ? () => onClick!(data.name, data.cleanName ?? data.name, data.playerId)
    : undefined

  return (
    <div
      className={`duelist ${clickable ? 'clickable' : ''}`}
      onClick={handleClick}
      role={clickable ? 'button' : undefined}
      tabIndex={clickable ? 0 : undefined}
      onKeyDown={clickable ? (e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          handleClick!()
        }
      } : undefined}
    >
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

export function Duelists({ left, right, state, live, onPlayerClick }: DuelistsProps) {
  const winnerSide = !live && state === 'left' ? 'left'
                    : !live && state === 'right' ? 'right' : null

  // Sized-up duelist columns leave no room for a stacked
  // "NO\nCONTEST" between them. Surface tie/no_contest as a thin
  // banner above the row instead; connector stays a clean "vs".
  const banner = !live && (state === 'tie' || state === 'no_contest') ? (
    <div className={`duelists-state ${state === 'tie' ? 'tie' : 'nc'}`}>
      {state === 'tie' ? 'TIE' : 'NO CONTEST'}
    </div>
  ) : null

  return (
    <>
      {banner}
      <div className="duelists">
        <Duelist data={left} side="left" winnerSide={winnerSide} onClick={onPlayerClick} />
        <span className="scoreboard__vs">vs</span>
        <Duelist data={right} side="right" winnerSide={winnerSide} onClick={onPlayerClick} />
        {/* Awards sit on row 2 directly below each duelist — grid
            placement is in CSS via .duelist-awards / .duelist-awards.right. */}
        <DuelistAwards awards={left.awards} side="left" />
        <DuelistAwards awards={right.awards} side="right" />
      </div>
    </>
  )
}
