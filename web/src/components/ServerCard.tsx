import { useState, useEffect, useRef } from 'react'
import type { ServerStatus, Player } from '../types'
import { FlagIcon } from './FlagIcon'
import { PlayerItem } from './PlayerItem'
import { formatNumber } from '../utils'
import { formatGameType } from './MatchCard'

interface ServerCardProps {
  server: ServerStatus
  newPlayers: Set<string>
  isSelected?: boolean
  onSelect?: () => void
  onPlayerClick?: (playerName: string, cleanName: string, playerId?: number) => void
}

// Check if this is a team-based game mode
function isTeamGame(gameType: string): boolean {
  const teamModes = ['team deathmatch', 'tdm', 'capture the flag', 'ctf', 'one flag ctf', 'overload', 'harvester']
  return teamModes.includes(gameType.toLowerCase())
}

// Check if this is CTF mode
function isCTF(gameType: string): boolean {
  const ctfModes = ['capture the flag', 'ctf', 'one flag ctf']
  return ctfModes.includes(gameType.toLowerCase())
}

// Get flag status indicator: 0=at base, 1=taken, 2=dropped
function getFlagIndicator(status: number): { className: string; title: string; status: 'base' | 'taken' | 'dropped' } {
  switch (status) {
    case 0:
      return { className: 'flag-base', title: 'At base', status: 'base' }
    case 1:
      return { className: 'flag-taken', title: 'Taken', status: 'taken' }
    case 2:
      return { className: 'flag-dropped', title: 'Dropped', status: 'dropped' }
    default:
      return { className: 'flag-base', title: 'Unknown', status: 'base' }
  }
}

// Get the relevant score limit for the game type
function getScoreLimit(gameType: string, serverVars?: Record<string, string>): number | null {
  if (!serverVars) return null

  if (isCTF(gameType)) {
    const limit = parseInt(serverVars.capturelimit || '0', 10)
    if (limit > 0) return limit
  } else {
    const limit = parseInt(serverVars.fraglimit || '0', 10)
    if (limit > 0) return limit
  }
  return null
}

// Get time limit if set
function getTimeLimit(serverVars?: Record<string, string>): number | null {
  if (!serverVars) return null
  const limit = parseInt(serverVars.timelimit || '0', 10)
  return limit > 0 ? limit : null
}

// Format game time from milliseconds to M:SS (supports negative for warmup)
function formatGameTime(ms: number, isWarmup: boolean = false): string {
  const absMs = Math.abs(ms)
  const totalSeconds = Math.floor(absMs / 1000)
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  const timeStr = `${minutes}:${seconds.toString().padStart(2, '0')}`
  return isWarmup ? `-${timeStr}` : timeStr
}

// Get effective game time, using warmup_remaining for negative countdown
function getDisplayTime(server: ServerStatus, timeLimitMinutes: number | null): { time: string; isWarmup: boolean; isOvertime: boolean } {
  const isWarmup = server.match_state === 'warmup' && server.warmup_remaining && server.warmup_remaining > 0
  if (isWarmup) {
    return { time: formatGameTime(server.warmup_remaining!, true), isWarmup: true, isOvertime: false }
  }
  const isOvertime = server.match_state === 'overtime'
  if (isOvertime && timeLimitMinutes) {
    const timeLimitMs = timeLimitMinutes * 60 * 1000
    const overtimeMs = server.game_time_ms - timeLimitMs
    return { time: `+${formatGameTime(Math.max(0, overtimeMs))}`, isWarmup: false, isOvertime: true }
  }
  return { time: formatGameTime(server.game_time_ms), isWarmup: false, isOvertime: false }
}


// Hook to interpolate game time between server updates
// timeLimitMinutes is used to clamp interpolated time so we don't falsely flag overtime
function useInterpolatedTime(server: ServerStatus, timeLimitMinutes: number | null): { gameTimeMs: number; warmupRemaining: number | undefined } {
  const [offset, setOffset] = useState(0)
  const lastUpdateRef = useRef<string>(server.last_updated)
  const baseGameTimeRef = useRef(server.game_time_ms)
  const baseWarmupRef = useRef(server.warmup_remaining)

  // Reset offset when server data changes
  useEffect(() => {
    if (server.last_updated !== lastUpdateRef.current) {
      lastUpdateRef.current = server.last_updated
      baseGameTimeRef.current = server.game_time_ms
      baseWarmupRef.current = server.warmup_remaining
      setOffset(0)
    }
  }, [server.last_updated, server.game_time_ms, server.warmup_remaining])

  // Increment/decrement timer every second based on match state
  useEffect(() => {
    // Only tick for active, overtime, or warmup states
    if (server.match_state !== 'active' && server.match_state !== 'overtime' && server.match_state !== 'warmup') {
      return
    }

    const interval = setInterval(() => {
      setOffset(prev => prev + 1000)
    }, 1000)

    return () => clearInterval(interval)
  }, [server.match_state, server.last_updated])

  // Calculate interpolated values
  let gameTimeMs = (server.match_state === 'active' || server.match_state === 'overtime')
    ? baseGameTimeRef.current + offset
    : server.game_time_ms

  // Clamp interpolated time at the time limit so we don't falsely flag overtime.
  // Overtime should only be shown when the actual server status reports it.
  if (timeLimitMinutes && timeLimitMinutes > 0 && server.match_state === 'active') {
    const timeLimitMs = timeLimitMinutes * 60 * 1000
    // Only clamp if the base time from server is still under the limit
    if (baseGameTimeRef.current <= timeLimitMs) {
      gameTimeMs = Math.min(gameTimeMs, timeLimitMs)
    }
  }

  const warmupRemaining = server.match_state === 'warmup' && baseWarmupRef.current !== undefined
    ? Math.max(0, baseWarmupRef.current - offset)
    : server.warmup_remaining

  return { gameTimeMs, warmupRemaining }
}

// Sort players by team (Red first, Blue second, then Spec, then Free/Unknown)
// Within each team, sort by score descending
function sortPlayersByTeam(players: Player[]): Player[] {
  return [...players].sort((a, b) => {
    const teamOrder = (team?: number) => {
      switch (team) {
        case 1: return 0  // Red first
        case 2: return 1  // Blue second
        case 3: return 2  // Spec third
        default: return 3 // Free/unknown last
      }
    }
    const teamDiff = teamOrder(a.team) - teamOrder(b.team)
    if (teamDiff !== 0) return teamDiff
    return b.score - a.score
  })
}

export function ServerCard({ server, newPlayers, isSelected, onSelect, onPlayerClick }: ServerCardProps) {
  const humans = server.players?.filter(p => !p.is_bot) ?? []
  const bots = server.players?.filter(p => p.is_bot) ?? []
  const showTeamScores = isTeamGame(server.game_type) && server.team_scores
  const scoreLimit = getScoreLimit(server.game_type, server.server_vars)
  const timeLimit = getTimeLimit(server.server_vars)

  // Use interpolated time for smooth updates between server reports
  const { gameTimeMs, warmupRemaining } = useInterpolatedTime(server, timeLimit)
  const interpolatedServer = { ...server, game_time_ms: gameTimeMs, warmup_remaining: warmupRemaining }
  const displayTime = getDisplayTime(interpolatedServer, timeLimit)
  // Determine state class and label for top-right badge
  const getStateBadge = (): { className: string; label: string } => {
    if (!server.online) {
      return { className: 'state-offline', label: 'Offline' }
    }
    switch (server.match_state) {
      case 'overtime':
        return { className: 'state-overtime', label: 'Overtime' }
      case 'warmup':
        return { className: 'state-warmup', label: 'Warmup' }
      case 'waiting':
        return { className: 'state-waiting', label: 'Waiting' }
      case 'intermission':
        return { className: 'state-intermission', label: 'Intermission' }
      case 'active':
      default:
        return { className: 'state-active', label: 'Active' }
    }
  }
  const stateBadge = getStateBadge()

  // Generate levelshot URL from map name
  const levelshotUrl = server.map ? `/assets/levelshots/${server.map.toLowerCase()}.jpg` : undefined

  return (
    <div
      className={`server-card ${isSelected ? 'selected' : ''} ${onSelect ? 'selectable' : ''}`}
      style={levelshotUrl ? { '--levelshot': `url(${levelshotUrl})` } as React.CSSProperties : undefined}
      onClick={onSelect}
      role={onSelect ? 'button' : undefined}
      tabIndex={onSelect ? 0 : undefined}
    >
      <span className="server-name-badge">
        <span className="badge-label">Server</span> {server.name}
        <span className="badge-sep">/</span>
        <span className="badge-label">Mode</span> {formatGameType(server.game_type)}
      </span>
      <span className={`server-state-badge ${stateBadge.className}`}>{stateBadge.label}</span>
      <div className="server-header">
        <span className="server-map">{server.map || 'Unknown'}</span>
        {(scoreLimit || timeLimit || gameTimeMs > 0 || displayTime.isWarmup) && (
          <span className="server-limits">
            {scoreLimit && <span className="score-limit">Limit: {scoreLimit}</span>}
            {scoreLimit && (timeLimit || gameTimeMs > 0 || displayTime.isWarmup) && <span className="limit-sep"> | </span>}
            <span className={displayTime.isOvertime ? 'overtime-time' : displayTime.isWarmup ? 'warmup-time' : ''}>{displayTime.time}</span>
            {timeLimit && <span className="time-limit"> / {timeLimit}m</span>}
          </span>
        )}
      </div>

      {showTeamScores && server.team_scores && (
        <div className="team-scores">
          <span className="team-score red">
            <span className="team-label">{server.server_vars?.g_redteam || 'Red'}</span>
            <span className="score-row">
              <span className="score-value">{formatNumber(server.team_scores.red)}</span>
              {server.flag_status && isCTF(server.game_type) && (() => {
                const indicator = getFlagIndicator(server.flag_status.red)
                return (
                  <span className={`flag-indicator ${indicator.className}`}>
                    <FlagIcon team="red" status={indicator.status} size="sm" title={`Red flag: ${indicator.title}`} />
                  </span>
                )
              })()}
            </span>
          </span>
          <span className="team-score blue">
            <span className="team-label">{server.server_vars?.g_blueteam || 'Blue'}</span>
            <span className="score-row">
              <span className="score-value">{formatNumber(server.team_scores.blue)}</span>
              {server.flag_status && isCTF(server.game_type) && (() => {
                const indicator = getFlagIndicator(server.flag_status.blue)
                return (
                  <span className={`flag-indicator ${indicator.className}`}>
                    <FlagIcon team="blue" status={indicator.status} size="sm" title={`Blue flag: ${indicator.title}`} />
                  </span>
                )
              })()}
            </span>
          </span>
        </div>
      )}

      <div className="player-counts">
        <span className="count-humans">{humans.length} humans</span>
        <span className="count-bots">{bots.length} bots</span>
      </div>

      <ul className="player-list">
        {server.players && server.players.length > 0 ? (
          sortPlayersByTeam(server.players).map((player, index) => (
            <PlayerItem
              key={`${player.clean_name}-${index}`}
              player={player}
              isNew={newPlayers.has(player.clean_name)}
              carryingFlag={
                server.flag_status?.red_carrier !== undefined &&
                server.flag_status.red_carrier >= 0 &&
                server.flag_status.red_carrier === player.client_num ? 'red' :
                server.flag_status?.blue_carrier !== undefined &&
                server.flag_status.blue_carrier >= 0 &&
                server.flag_status.blue_carrier === player.client_num ? 'blue' :
                undefined
              }
              onClick={onPlayerClick ? () => onPlayerClick(player.name, player.clean_name, player.player_id) : undefined}
            />
          ))
        ) : (
          <li className="no-players">No players</li>
        )}
      </ul>
    </div>
  )
}
