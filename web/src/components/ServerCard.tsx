import { useState, useEffect } from 'react'
import type { ServerStatus, Player, FlagStatus } from '../types'
import { FlagIcon } from './FlagIcon'
import { PlayerItem } from './PlayerItem'
import { formatNumber, serverDisplay } from '../utils'
import { useSources } from '../hooks/useSources'
import { formatGameType } from './MatchCard'

interface ServerCardProps {
  server: ServerStatus
  newPlayers: Set<string>
  isSelected?: boolean
  onSelect?: () => void
  onPlayerClick?: (playerName: string, cleanName: string, playerId?: number) => void
  liveness?: 'live' | 'stale' | 'offline'
}

// Check if this is a team-based game mode
function isTeamGame(gameType: string): boolean {
  const teamModes = ['team deathmatch', 'tdm', 'capture the flag', 'ctf', 'one flag ctf', '1fctf', 'overload', 'harvester']
  return teamModes.includes(gameType.toLowerCase())
}

// Check if this is CTF mode
function isCTF(gameType: string): boolean {
  const ctfModes = ['capture the flag', 'ctf', 'one flag ctf', '1fctf']
  return ctfModes.includes(gameType.toLowerCase())
}

// In 1FCTF only neutral_carrier is meaningful; in CTF only red/blue.
// The unused fields are present-but-meaningless and must not be matched.
function flagCarriedBy(fs: FlagStatus | undefined, clientNum: number): 'red' | 'blue' | 'neutral' | undefined {
  if (!fs) return undefined
  if (fs.mode === '1fctf') {
    return fs.neutral_carrier !== undefined && fs.neutral_carrier === clientNum ? 'neutral' : undefined
  }
  if (fs.red_carrier === clientNum && fs.red_carrier >= 0) return 'red'
  if (fs.blue_carrier === clientNum && fs.blue_carrier >= 0) return 'blue'
  return undefined
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

// 1FCTF status values: 0=at base, 2=carried by red, 3=carried by blue,
// 4=dropped. The single neutral flag drifts toward whichever side
// currently carries it.
function getNeutralFlagIndicator(status: number): { drift: string; title: string; status: 'base' | 'taken' | 'dropped' } {
  switch (status) {
    case 0:
      return { drift: 'flag-center', title: 'Neutral flag at base', status: 'base' }
    case 2:
      return { drift: 'flag-toward-red', title: 'Neutral flag carried by red', status: 'taken' }
    case 3:
      return { drift: 'flag-toward-blue', title: 'Neutral flag carried by blue', status: 'taken' }
    case 4:
      return { drift: 'flag-center', title: 'Neutral flag dropped', status: 'dropped' }
    default:
      return { drift: 'flag-center', title: 'Neutral flag', status: 'base' }
  }
}

// usesCaptureLimit returns true for the gametypes that score on
// captures/objectives rather than frags. Mirrors the engine's
// `g_gametype >= GT_CTF` check (g_main.c) — CTF, 1FCTF, Overload,
// and Harvester all read g_capturelimit, not g_fraglimit.
function usesCaptureLimit(gameType: string): boolean {
  if (isCTF(gameType)) return true
  const gt = gameType.toLowerCase()
  return gt === 'overload' || gt === 'harvester'
}

// Get the relevant score limit for the game type
function getScoreLimit(gameType: string, serverVars?: Record<string, string>): number | null {
  if (!serverVars) return null

  if (usesCaptureLimit(gameType)) {
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
  // Track the snapshot we're interpolating from alongside the offset, so when
  // a new snapshot arrives we can reset offset during render — see
  // https://react.dev/reference/react/useState#storing-information-from-previous-renders.
  const [state, setState] = useState({ lastUpdated: server.last_updated, offset: 0 })

  if (state.lastUpdated !== server.last_updated) {
    setState({ lastUpdated: server.last_updated, offset: 0 })
  }

  // Increment/decrement timer every second based on match state
  useEffect(() => {
    // Only tick for active, overtime, or warmup states
    if (server.match_state !== 'active' && server.match_state !== 'overtime' && server.match_state !== 'warmup') {
      return
    }

    const interval = setInterval(() => {
      setState(s => ({ ...s, offset: s.offset + 1000 }))
    }, 1000)

    return () => clearInterval(interval)
  }, [server.match_state, server.last_updated])

  // Use a stable offset only after the reset has landed — for the one render
  // where we still hold the previous snapshot's offset, treat it as zero.
  const offset = state.lastUpdated === server.last_updated ? state.offset : 0

  // Calculate interpolated values
  let gameTimeMs = (server.match_state === 'active' || server.match_state === 'overtime')
    ? server.game_time_ms + offset
    : server.game_time_ms

  // Clamp interpolated time at the time limit so we don't falsely flag overtime.
  // Overtime should only be shown when the actual server status reports it.
  if (timeLimitMinutes && timeLimitMinutes > 0 && server.match_state === 'active') {
    const timeLimitMs = timeLimitMinutes * 60 * 1000
    if (server.game_time_ms <= timeLimitMs) {
      gameTimeMs = Math.min(gameTimeMs, timeLimitMs)
    }
  }

  const warmupRemaining = server.match_state === 'warmup' && server.warmup_remaining !== undefined
    ? Math.max(0, server.warmup_remaining - offset)
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

export const MOVEMENT_MODES: Record<string, { icon: string; label: string }> = {
  '0': { icon: '/assets/modes/vq3.png', label: 'Quake 3' },
  '1': { icon: '/assets/modes/cpm.png', label: 'CPMA' },
  '2': { icon: '/assets/modes/ql.png', label: 'Quake Live' },
  '3': { icon: '/assets/modes/qlt.png', label: 'Quake Live Turbo' },
}

export const GAMEPLAY_MODES: Record<string, { icon: string; label: string }> = {
  '0': { icon: '/assets/modes/vq3.png', label: 'Quake 3' },
  '1': { icon: '/assets/modes/cpm.png', label: 'CPMA' },
  '2': { icon: '/assets/modes/ql.png', label: 'Quake Live' },
}

export function ModeIcons({ movement, gameplay }: { movement?: string, gameplay?: string }) {
  const moveMode = MOVEMENT_MODES[movement ?? '0']
  const gameMode = GAMEPLAY_MODES[gameplay ?? '0']
  if (!moveMode && !gameMode) return null
  return (
    <span className="mode-icons">
      <span className="mode-label">M</span>
      {moveMode && <img className="mode-icon" src={moveMode.icon} alt={moveMode.label} />}
      <span className="mode-label">G</span>
      {gameMode && <img className="mode-icon" src={gameMode.icon} alt={gameMode.label} />}
      <span className="mode-panel">
        {moveMode && (
          <span className="mode-panel-row">
            <img src={moveMode.icon} alt={moveMode.label} />
            <span>Movement: {moveMode.label}</span>
          </span>
        )}
        {gameMode && (
          <span className="mode-panel-row">
            <img src={gameMode.icon} alt={gameMode.label} />
            <span>Gameplay: {gameMode.label}</span>
          </span>
        )}
      </span>
    </span>
  )
}

export function ServerCard({ server, newPlayers, isSelected, onSelect, onPlayerClick, liveness }: ServerCardProps) {
  const { hasMultiple: hasMultipleSources } = useSources()
  const humans = server.players?.filter(p => !p.is_bot) ?? []
  const bots = server.players?.filter(p => p.is_bot) ?? []
  const showTeamScores = isTeamGame(server.game_type) && server.team_scores
  const scoreLimit = getScoreLimit(server.game_type, server.server_vars)
  const timeLimit = getTimeLimit(server.server_vars)

  // Use interpolated time for smooth updates between server reports
  const { gameTimeMs, warmupRemaining } = useInterpolatedTime(server, timeLimit)
  const interpolatedServer = { ...server, game_time_ms: gameTimeMs, warmup_remaining: warmupRemaining }
  const displayTime = getDisplayTime(interpolatedServer, timeLimit)
  // Determine state class and label for top-right badge.
  // Liveness from /api/servers wins over match_state when degraded — a
  // stale collector or UDP-unreachable server should signal that
  // first, since match-state values like "active" become misleading
  // when the data behind them is no longer fresh.
  const getStateBadge = (): { className: string; label: string } => {
    if (liveness === 'offline' || !server.online) {
      return { className: 'state-offline', label: 'Offline' }
    }
    if (liveness === 'stale') {
      return { className: 'state-stale', label: 'Stale' }
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
        <ModeIcons movement={server.server_vars?.g_movement} gameplay={server.server_vars?.g_gameplay} />
        {serverDisplay(server.source, server.key, { hasMultipleSources: hasMultipleSources })}
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
              {server.flag_status?.mode === 'ctf' && (() => {
                const indicator = getFlagIndicator(server.flag_status.red)
                return (
                  <span className={`flag-indicator ${indicator.className}`}>
                    <FlagIcon team="red" status={indicator.status} size="sm" title={`Red flag: ${indicator.title}`} />
                  </span>
                )
              })()}
            </span>
          </span>
          {server.flag_status?.mode === '1fctf' && (() => {
            const indicator = getNeutralFlagIndicator(server.flag_status.neutral ?? 0)
            return (
              <span className={`team-score team-flag-center ${indicator.drift}`}>
                <span className="team-label" aria-hidden="true">&nbsp;</span>
                <span className="score-row">
                  <span className="score-value" aria-hidden="true">&nbsp;</span>
                  <span className="flag-indicator">
                    <FlagIcon team="neutral" status={indicator.status} size="sm" title={indicator.title} />
                  </span>
                </span>
              </span>
            )
          })()}
          <span className="team-score blue">
            <span className="team-label">{server.server_vars?.g_blueteam || 'Blue'}</span>
            <span className="score-row">
              <span className="score-value">{formatNumber(server.team_scores.blue)}</span>
              {server.flag_status?.mode === 'ctf' && (() => {
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
              carryingFlag={flagCarriedBy(server.flag_status, player.client_num)}
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
