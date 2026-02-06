import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import type { MatchSummary, MatchPlayerSummary } from '../types'
import { BotBadge } from './BotBadge'
import { ColoredText } from './ColoredText'
import { MedalIcon } from './MedalIcon'
import { PlayerPortrait } from './PlayerPortrait'
import { PlayerBadge } from './PlayerBadge'
import { formatNumber, stripVRPrefix } from '../utils'

export function formatDuration(startedAt: string, endedAt: string): string {
  const start = new Date(startedAt)
  const end = new Date(endedAt)
  const diffMs = end.getTime() - start.getTime()
  const totalSecs = Math.floor(diffMs / 1000)
  const mins = Math.floor(totalSecs / 60)
  const secs = totalSecs % 60
  return `${mins}:${secs.toString().padStart(2, '0')}`
}

export function formatTimeAgo(dateString: string): string {
  const date = new Date(dateString)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMins = Math.floor(diffMs / 60000)

  if (diffMins < 1) return 'just now'
  if (diffMins < 60) return `${diffMins}m ago`

  const diffHours = Math.floor(diffMins / 60)
  if (diffHours < 24) return `${diffHours}h ago`

  const diffDays = Math.floor(diffHours / 24)
  return `${diffDays}d ago`
}

export function formatGameType(gameType: string): string {
  if (!gameType) return '?'
  const gt = gameType.toLowerCase()
  if (gt === 'free for all' || gt === 'ffa') return 'FFA'
  if (gt === 'team deathmatch' || gt === 'tdm') return 'TDM'
  if (gt === 'capture the flag' || gt === 'ctf') return 'CTF'
  if (gt === 'tournament' || gt === '1v1') return '1v1'
  return gameType.slice(0, 3).toUpperCase()
}

export function isTeamGame(gameType: string): boolean {
  if (!gameType) return false
  const gt = gameType.toLowerCase()
  return gt === 'team deathmatch' || gt === 'tdm' ||
         gt === 'capture the flag' || gt === 'ctf' ||
         gt === 'one flag ctf' || gt === 'overload' || gt === 'harvester'
}

function getTeamClass(team?: number): string {
  if (team === 1) return 'team-red'
  if (team === 2) return 'team-blue'
  return ''
}

interface MatchCardProps {
  match: MatchSummary
  onPlayerClick?: (playerName: string, cleanName: string, playerId?: number) => void
  highlightPlayerId?: number
  showPermalink?: boolean
}

export function MatchCard({ match, onPlayerClick, highlightPlayerId, showPermalink = false }: MatchCardProps) {
  const isTeam = isTeamGame(match.game_type)
  const players = [...(match.players ?? [])].sort((a, b) => {
    // Sort by team first (red=1, blue=2, others after), then by score descending
    const teamA = a.team ?? 99
    const teamB = b.team ?? 99
    if (teamA !== teamB) return teamA - teamB
    return (b.score ?? 0) - (a.score ?? 0)
  })

  const levelshotUrl = match.map_name ? `/assets/levelshots/${match.map_name.toLowerCase()}.jpg` : undefined

  // Helper to check if player is winner
  const isPlayerWinner = (player: MatchPlayerSummary) => {
    return (player.victories ?? 0) > 0
  }

  return (
    <div
      className="match-card"
      style={levelshotUrl ? { '--levelshot': `url(${levelshotUrl})` } as React.CSSProperties : undefined}
    >
      <div className="match-header">
        <div className="match-title">
          <span className="match-map">{match.map_name}</span>
          <span className="match-gametype">{formatGameType(match.game_type)}</span>
        </div>
        <div className="match-header-right">
          {match.ended_at && (
            <div className="match-timing">
              <span className="match-ago" title={new Date(match.ended_at).toLocaleString()}>{formatTimeAgo(match.ended_at)}</span>
              <span className="match-duration">{formatDuration(match.started_at, match.ended_at)}</span>
            </div>
          )}
          {showPermalink && (
            <Link
              to={`/matches/${match.id}`}
              className="permalink-btn"
              title="Permalink to this match"
              onClick={(e) => e.stopPropagation()}
            >
              #
            </Link>
          )}
        </div>
      </div>

      {isTeam && match.red_score != null && match.blue_score != null && (
        <div className="team-scores">
          <div className="team-scores-inner">
            {match.red_score > match.blue_score && (
              <span className="victory-badge left">
                <MedalIcon type="victory" size="lg" showCount={false} />
              </span>
            )}
            <div className="team-score red">
              <span className="team-label">Red</span>
              <span className="score-value">{formatNumber(match.red_score)}</span>
            </div>
            <div className="team-score blue">
              <span className="team-label">Blue</span>
              <span className="score-value">{formatNumber(match.blue_score)}</span>
            </div>
            {match.blue_score > match.red_score && (
              <span className="victory-badge right">
                <MedalIcon type="victory" size="lg" showCount={false} />
              </span>
            )}
          </div>
        </div>
      )}

      {!isTeam && players.length > 0 && (() => {
        const winners = players.filter(p => (p.victories ?? 0) > 0)
        return (
          <div className="ffa-winners">
            {winners.map((winner, idx) => (
              <div key={`winner-${winner.player_id}-${idx}`} className="ffa-winner-row">
                <PlayerPortrait model={winner.model} size="md" />
                {winner.is_bot && <BotBadge isBot skill={winner.skill!} size="md" />}
                {!winner.is_bot && <PlayerBadge playerId={winner.player_id} isVR={winner.is_vr} size="md" />}
                <ColoredText text={winner.is_vr ? stripVRPrefix(winner.name) : winner.name} />
              </div>
            ))}
          </div>
        )
      })()}

      <ul className="match-player-list">
        {players.map((player, index) => (
          <MatchPlayerRow
            key={`${player.player_id}-${index}`}
            player={player}
            showTeam={isTeam}
            isWinner={isPlayerWinner(player)}
            highlightPlayerId={highlightPlayerId}
            onPlayerClick={onPlayerClick}
          />
        ))}
      </ul>
    </div>
  )
}

interface MatchPlayerRowProps {
  player: MatchPlayerSummary
  showTeam?: boolean
  isWinner?: boolean
  highlightPlayerId?: number
  onPlayerClick?: (playerName: string, cleanName: string, playerId?: number) => void
}

function MatchPlayerRow({ player, showTeam, isWinner, highlightPlayerId, onPlayerClick }: MatchPlayerRowProps) {
  const teamClass = showTeam ? getTeamClass(player.team) : ''
  const isHighlighted = highlightPlayerId === player.player_id
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

  return (
    <li
      className={`match-player-row ${teamClass} ${onPlayerClick ? 'clickable' : ''} ${isHighlighted ? 'highlighted' : ''}`}
      onClick={onPlayerClick ? (e: React.MouseEvent) => { e.stopPropagation(); onPlayerClick(player.name, player.clean_name, player.player_id) } : undefined}
    >
      <span className="player-name">
        <span className={`completion-dot ${player.completed ? 'completed' : 'left-early'}`} />
        <PlayerPortrait model={player.model} size="sm" />
        {player.is_bot && <BotBadge isBot skill={player.skill!} />}
        {!player.is_bot && <PlayerBadge playerId={player.player_id} isVR={player.is_vr} />}
        <ColoredText text={player.is_vr ? stripVRPrefix(player.name) : player.name} />
        <span
          ref={awardsRef}
          className={`awards-container ${isOverflowing ? 'overflowing' : ''}`}
          style={isOverflowing ? { '--scroll-width': `${scrollWidth}px` } as React.CSSProperties : undefined}
        >
          {isWinner && (
            <MedalIcon type="victory" showCount={false} />
          )}
          {(player.impressives ?? 0) > 0 && (
            <MedalIcon type="impressive" count={player.impressives} />
          )}
          {(player.excellents ?? 0) > 0 && (
            <MedalIcon type="excellent" count={player.excellents} />
          )}
          {(player.humiliations ?? 0) > 0 && (
            <MedalIcon type="humiliation" count={player.humiliations} />
          )}
          {(player.defends ?? 0) > 0 && (
            <MedalIcon type="defend" count={player.defends} />
          )}
          {(player.captures ?? 0) > 0 && (
            <MedalIcon type="capture" count={player.captures} />
          )}
          {(player.assists ?? 0) > 0 && (
            <MedalIcon type="assist" count={player.assists} />
          )}
        </span>
      </span>
      <span className="player-stats">
        <span className="kd">
          <span className="frags">{formatNumber(player.frags)}</span>
          <span className="sep">/</span>
          <span className="deaths">{formatNumber(player.deaths)}</span>
        </span>
        {player.score != null && (
          <span className="score">{formatNumber(player.score)}</span>
        )}
      </span>
    </li>
  )
}
