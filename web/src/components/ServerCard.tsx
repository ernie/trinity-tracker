import { useState, useEffect, useRef } from 'react'
import type { ServerStatus, Player } from '../types'
import { FlagIcon } from './FlagIcon'
import { PlayerItem } from './PlayerItem'

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

// Check if match is in overtime (time exceeded but still active)
function getOvertimeInfo(gameTimeMs: number, timeLimitMinutes: number | null, matchState?: string) {
  if (!timeLimitMinutes || timeLimitMinutes <= 0 || matchState !== 'active') {
    return { isOvertime: false, overtimeMs: 0 }
  }
  const timeLimitMs = timeLimitMinutes * 60 * 1000
  const isOvertime = gameTimeMs > timeLimitMs
  return {
    isOvertime,
    overtimeMs: isOvertime ? gameTimeMs - timeLimitMs : 0
  }
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
function getDisplayTime(server: ServerStatus, timeLimitMinutes: number | null): { time: string; isWarmup: boolean; isOvertime: boolean; overtimeMs: number } {
  const isWarmup = server.match_state === 'warmup' && server.warmup_remaining && server.warmup_remaining > 0
  if (isWarmup) {
    return { time: formatGameTime(server.warmup_remaining!, true), isWarmup: true, isOvertime: false, overtimeMs: 0 }
  }
  const overtime = getOvertimeInfo(server.game_time_ms, timeLimitMinutes, server.match_state)
  if (overtime.isOvertime) {
    return { time: `+${formatGameTime(overtime.overtimeMs)}`, isWarmup: false, isOvertime: true, overtimeMs: overtime.overtimeMs }
  }
  return { time: formatGameTime(server.game_time_ms), isWarmup: false, isOvertime: false, overtimeMs: 0 }
}

// Get match state badge class and label
function getMatchStateBadge(state?: string): { className: string; label: string } | null {
  switch (state) {
    case 'warmup':
      return { className: 'match-state-warmup', label: 'Warmup' }
    case 'waiting':
      return { className: 'match-state-waiting', label: 'Waiting' }
    case 'intermission':
      return { className: 'match-state-intermission', label: 'Intermission' }
    case 'active':
      return null // Don't show badge for active - it's the normal state
    default:
      return null
  }
}

// Hook to interpolate game time between server updates
function useInterpolatedTime(server: ServerStatus): { gameTimeMs: number; warmupRemaining: number | undefined } {
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
    // Only tick for active or warmup states
    if (server.match_state !== 'active' && server.match_state !== 'warmup') {
      return
    }

    const interval = setInterval(() => {
      setOffset(prev => prev + 1000)
    }, 1000)

    return () => clearInterval(interval)
  }, [server.match_state, server.last_updated])

  // Calculate interpolated values
  const gameTimeMs = server.match_state === 'active'
    ? baseGameTimeRef.current + offset
    : server.game_time_ms

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
  const matchStateBadge = getMatchStateBadge(server.match_state)

  // Use interpolated time for smooth updates between server reports
  const { gameTimeMs, warmupRemaining } = useInterpolatedTime(server)
  const interpolatedServer = { ...server, game_time_ms: gameTimeMs, warmup_remaining: warmupRemaining }
  const displayTime = getDisplayTime(interpolatedServer, timeLimit)

  // Determine dot color and tooltip based on server state
  const getStatusInfo = (): { className: string; tooltip: string } => {
    if (!server.online) {
      return { className: 'offline', tooltip: 'Server Offline' }
    }
    switch (server.match_state) {
      case 'warmup':
        return { className: 'warmup', tooltip: 'Warmup' }
      case 'waiting':
        return { className: 'waiting', tooltip: 'Waiting for Players' }
      case 'intermission':
        return { className: 'intermission', tooltip: 'Intermission' }
      case 'active':
      default:
        return { className: 'online', tooltip: 'In Progress' }
    }
  }
  const statusInfo = getStatusInfo()

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
      <div className="server-header">
        <span className="server-name">{server.name}</span>
        <span className="server-info">
          <span className={`status-dot ${statusInfo.className}`} title={statusInfo.tooltip} />
          <span className="server-map">{server.map || 'Unknown'}</span>
          {matchStateBadge && (
            <span className={`match-state-badge ${matchStateBadge.className}`} title={matchStateBadge.label}>
              {matchStateBadge.label}
            </span>
          )}
        </span>
      </div>

      {showTeamScores && server.team_scores && (
        <div className="team-scores">
          <span className="team-score red">
            <span className="team-label">{server.server_vars?.g_redteam || 'Red'}</span>
            <span className="score-row">
              <span className="score-value">{server.team_scores.red}</span>
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
              <span className="score-value">{server.team_scores.blue}</span>
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
          <span className="score-limit">
            {scoreLimit && <>/ {scoreLimit}</>}
            {scoreLimit && timeLimit && ' | '}
            {timeLimit && displayTime.isOvertime && (
              <><span className="overtime-indicator">OT</span><span className="overtime-time">{displayTime.time}</span></>
            )}
            {timeLimit && !displayTime.isOvertime && (
              <><span className={displayTime.isWarmup ? 'warmup-time' : ''}>{displayTime.time}</span> / {timeLimit} min</>
            )}
            {!timeLimit && (gameTimeMs > 0 || displayTime.isWarmup) && <>{scoreLimit && ' | '}<span className={displayTime.isWarmup ? 'warmup-time' : ''}>{displayTime.time}</span></>}
          </span>
        </div>
      )}

      {!showTeamScores && (scoreLimit || timeLimit || gameTimeMs > 0 || displayTime.isWarmup) && (
        <div className="game-limits">
          {scoreLimit && <span>{scoreLimit}</span>}
          {scoreLimit && (timeLimit || gameTimeMs > 0 || displayTime.isWarmup) && <span className="limit-separator"> | </span>}
          {timeLimit && displayTime.isOvertime && (
            <span><span className="overtime-indicator">OT</span><span className="overtime-time">{displayTime.time}</span></span>
          )}
          {timeLimit && !displayTime.isOvertime && (
            <span className={displayTime.isWarmup ? 'warmup-time' : ''}>{displayTime.time} / {timeLimit} min</span>
          )}
          {!timeLimit && (gameTimeMs > 0 || displayTime.isWarmup) && <span className={displayTime.isWarmup ? 'warmup-time' : ''}>{displayTime.time}</span>}
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
