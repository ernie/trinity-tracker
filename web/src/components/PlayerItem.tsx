import { useEffect, useRef, useState } from 'react'
import type { Player } from '../types'
import { BotBadge } from './BotBadge'
import { ColoredText } from './ColoredText'
import { FlagIcon } from './FlagIcon'
import { MedalIcon } from './MedalIcon'
import { PlayerPortrait } from './PlayerPortrait'
import { PlayerBadge } from './PlayerBadge'
import { stripVRPrefix } from '../utils'

interface PlayerItemProps {
  player: Player
  isNew?: boolean
  carryingFlag?: 'red' | 'blue'
  onClick?: () => void
}

// Q3 team values: 0 = Free, 1 = Red, 2 = Blue, 3 = Spectator
function getTeamClass(team?: number): string {
  switch (team) {
    case 1: return 'team-red'
    case 2: return 'team-blue'
    case 3: return 'team-spec'
    default: return ''
  }
}

// Format time in match from joined_at timestamp to M:SS
function formatTimeInMatch(joinedAt?: string): string | null {
  if (!joinedAt) return null
  const joined = new Date(joinedAt)
  // Ignore zero/invalid timestamps (Go zero time is year 0001)
  if (joined.getFullYear() < 2000) return null
  const now = new Date()
  const diffMs = now.getTime() - joined.getTime()
  if (diffMs < 0) return null
  const totalSeconds = Math.floor(diffMs / 1000)
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  return `${minutes}:${seconds.toString().padStart(2, '0')}`
}

export function PlayerItem({ player, isNew, carryingFlag, onClick }: PlayerItemProps) {
  const teamClass = getTeamClass(player.team)
  const isClickable = onClick != null
  const itemClasses = ['player-item', teamClass, isNew ? 'new-player' : '', isClickable ? 'clickable' : ''].filter(Boolean).join(' ')
  const nameClasses = 'player-name'
  const timeInMatch = formatTimeInMatch(player.joined_at)
  const awardsRef = useRef<HTMLSpanElement>(null)
  const [isOverflowing, setIsOverflowing] = useState(false)
  const [scrollWidth, setScrollWidth] = useState(0)

  useEffect(() => {
    const checkOverflow = () => {
      if (awardsRef.current) {
        const sw = awardsRef.current.scrollWidth
        const cw = awardsRef.current.clientWidth
        setIsOverflowing(sw > cw)
        setScrollWidth(sw)
      }
    }
    checkOverflow()
    window.addEventListener('resize', checkOverflow)
    return () => window.removeEventListener('resize', checkOverflow)
  }, [])

  const handleClick = (e: React.MouseEvent) => {
    if (isClickable) {
      e.stopPropagation()
      onClick()
    }
  }

  return (
    <li className={itemClasses} onClick={handleClick}>
      <span className={nameClasses}>
        <PlayerPortrait model={player.model} size="sm" />
        {player.is_bot && <BotBadge isBot skill={player.skill!} />}
        {!player.is_bot && <PlayerBadge playerId={player.player_id!} isVR={player.is_vr} />}
        <ColoredText text={player.is_vr ? stripVRPrefix(player.name) : player.name} />
        {carryingFlag && (
          <span className="player-flag">
            <FlagIcon team={carryingFlag} status="base" size="sm" title={`Carrying ${carryingFlag} flag`} />
          </span>
        )}
        <span
          ref={awardsRef}
          className={`awards-container ${isOverflowing ? 'overflowing' : ''}`}
          style={isOverflowing ? { '--scroll-width': `${scrollWidth}px` } as React.CSSProperties : undefined}
        >
          {player.impressives && player.impressives > 0 && (
            <MedalIcon type="impressive" count={player.impressives} />
          )}
          {player.excellents && player.excellents > 0 && (
            <MedalIcon type="excellent" count={player.excellents} />
          )}
          {player.humiliations && player.humiliations > 0 && (
            <MedalIcon type="humiliation" count={player.humiliations} />
          )}
          {player.defends && player.defends > 0 && (
            <MedalIcon type="defend" count={player.defends} />
          )}
          {player.captures && player.captures > 0 && (
            <MedalIcon type="capture" count={player.captures} />
          )}
          {player.assists && player.assists > 0 && (
            <MedalIcon type="assist" count={player.assists} />
          )}
        </span>
      </span>
      <span className="player-stats">
        <span className="player-score">{player.score}</span>
        {timeInMatch && <span className="player-time" title="Time in match">{timeInMatch}</span>}
        <span className="player-ping">{player.ping}ms</span>
      </span>
    </li>
  )
}
