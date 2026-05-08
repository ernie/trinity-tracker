import { useState, useEffect } from 'react'
import type { ServerStatus, Player } from '../types'
import { FlagIcon } from './FlagIcon'
import { useSources } from '../hooks/useSources'
import { formatGameType } from './MatchCard'
import { useMapMeta } from '../hooks/useMapMeta'
import { RichChip } from './cards/RichChip'
import { Scoreboard } from './cards/Scoreboard'
import { Duelists, type DuelistData } from './cards/Duelists'
import { classifyScores, awardsFromCounts } from './cards/format'
import { PlayerRows } from './cards/PlayerRows'
import { SpectatorStrip } from './cards/SpectatorStrip'

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
// `live` gates the local 1-second tick: when the server is offline or its
// collector is stale the poller keeps re-broadcasting the same snapshot
// every poll cycle, so ticking would just produce a saw-tooth (count up
// 1s per second, snap back when the next stale broadcast lands).
function useInterpolatedTime(server: ServerStatus, timeLimitMinutes: number | null, live: boolean): { gameTimeMs: number; warmupRemaining: number | undefined } {
  // Track the snapshot we're interpolating from alongside the offset, so when
  // a new snapshot arrives we can reset offset during render — see
  // https://react.dev/reference/react/useState#storing-information-from-previous-renders.
  const [state, setState] = useState({ lastUpdated: server.last_updated, offset: 0 })

  if (state.lastUpdated !== server.last_updated) {
    setState({ lastUpdated: server.last_updated, offset: 0 })
  }

  // Increment/decrement timer every second based on match state
  useEffect(() => {
    if (!live) return
    // Only tick for active, overtime, or warmup states
    if (server.match_state !== 'active' && server.match_state !== 'overtime' && server.match_state !== 'warmup') {
      return
    }

    const interval = setInterval(() => {
      setState(s => ({ ...s, offset: s.offset + 1000 }))
    }, 1000)

    return () => clearInterval(interval)
  }, [server.match_state, server.last_updated, live])

  // Use a stable offset only after the reset has landed — for the one render
  // where we still hold the previous snapshot's offset, treat it as zero.
  const offset = state.lastUpdated === server.last_updated ? state.offset : 0

  // Calculate interpolated values. During warmup the engine's
  // game_time_ms is time-since-map-load (it includes warmup), so falling
  // through to it once the countdown hits 0 would briefly flash ~warmup
  // duration as "active" time. Synthesize the active clock locally
  // (offset past warmup_remaining) for a smooth handoff.
  let gameTimeMs: number
  if (server.match_state === 'active' || server.match_state === 'overtime') {
    gameTimeMs = server.game_time_ms + offset
  } else if (server.match_state === 'warmup' && server.warmup_remaining !== undefined) {
    gameTimeMs = Math.max(0, offset - server.warmup_remaining)
  } else {
    gameTimeMs = server.game_time_ms
  }

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

export function ServerCard({ server, newPlayers: _newPlayers, isSelected, onSelect, onPlayerClick, liveness }: ServerCardProps) {
  const { hasMultiple: hasMultipleSources } = useSources()
  const meta = useMapMeta(server.map)
  const showTeamScores = isTeamGame(server.game_type) && server.team_scores
  const scoreLimit = getScoreLimit(server.game_type, server.server_vars)
  const timeLimit = getTimeLimit(server.server_vars)

  // A card is "degraded" when its data is no longer trustworthy: the
  // UDP poll has been failing (offline) or the collector heartbeat has
  // aged out (stale). We freeze the local clock and dim the card so
  // the timer doesn't saw-tooth and the staleness reads visually.
  // `liveness` is undefined on first render (before /api/servers
  // returns) — treat that as live so we don't briefly dim healthy cards.
  const isDegraded = liveness === 'offline' || liveness === 'stale' || server.online === false

  // Use interpolated time for smooth updates between server reports
  const { gameTimeMs, warmupRemaining } = useInterpolatedTime(server, timeLimit, !isDegraded)
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
        return { className: 'overtime', label: 'Overtime' }
      case 'warmup':
        return { className: 'warmup', label: 'Warmup' }
      case 'waiting':
        return { className: 'waiting', label: 'Waiting' }
      case 'intermission':
        return { className: 'intermission', label: 'Intermission' }
      case 'active':
      default:
        return { className: 'active', label: 'Active' }
    }
  }
  const stateBadge = getStateBadge()

  // Generate levelshot URL from map name
  const levelshotUrl = server.map ? `/assets/levelshots/${server.map.toLowerCase()}.jpg` : undefined

  // Players excluding spectators (team 3), sorted for display
  const activePlayers = sortPlayersByTeam(
    (server.players ?? []).filter(p => p.team !== 3)
  )
  const spectators = (server.players ?? []).filter(p => p.team === 3)

  return (
    <div
      className={`card server-card ${isSelected ? 'selected' : ''} ${onSelect ? 'selectable' : ''} ${isDegraded ? 'degraded' : ''}`}
      style={levelshotUrl ? { '--levelshot': `url(${levelshotUrl})` } as React.CSSProperties : undefined}
      onClick={onSelect}
      role={onSelect ? 'button' : undefined}
      tabIndex={onSelect ? 0 : undefined}
    >
      <div className="card__topbar">
        <RichChip
          source={hasMultipleSources ? server.source : undefined}
          server={server.key}
          mode={formatGameType(server.game_type)}
        />
        <ModeIcons movement={server.server_vars?.g_movement} gameplay={server.server_vars?.g_gameplay} />
        <span className={`card__state ${stateBadge.className}`}>{stateBadge.label}</span>
      </div>

      <div className="card__header">
        <div className="card__map">
          {meta.longName && <span className="card__map-short">{meta.shortName}</span>}
          <span className="card__map-long">{meta.displayName || server.map || 'Unknown'}</span>
        </div>
        {(scoreLimit || timeLimit || gameTimeMs > 0 || displayTime.isWarmup) && (
          <div className="card__limits">
            {scoreLimit && <span>Limit: {scoreLimit}</span>}
            {scoreLimit && (timeLimit || gameTimeMs > 0 || displayTime.isWarmup) && <span> | </span>}
            <span className={displayTime.isOvertime ? 'overtime-time' : displayTime.isWarmup ? 'warmup-time' : ''}>{displayTime.time}</span>
            {timeLimit && <span> / {timeLimit}m</span>}
          </div>
        )}
      </div>

      {showTeamScores && server.team_scores && (() => {
        // CTF/1FCTF feed flag icons into Scoreboard slots; other team modes leave them empty.
        const flag = server.flag_status
        const redInd = flag?.mode === 'ctf' ? (() => {
          const i = getFlagIndicator(flag.red)
          return (
            <span className={`flag-indicator ${i.className}`}>
              <FlagIcon team="red" status={i.status} size="sm" title={`Red flag: ${i.title}`} />
            </span>
          )
        })() : undefined
        const blueInd = flag?.mode === 'ctf' ? (() => {
          const i = getFlagIndicator(flag.blue)
          return (
            <span className={`flag-indicator ${i.className}`}>
              <FlagIcon team="blue" status={i.status} size="sm" title={`Blue flag: ${i.title}`} />
            </span>
          )
        })() : undefined
        const centerInd = flag?.mode === '1fctf' ? (() => {
          const i = getNeutralFlagIndicator(flag.neutral ?? 0)
          return (
            <span className={`flag-indicator ${i.drift}`}>
              <FlagIcon team="neutral" status={i.status} size="sm" title={i.title} />
            </span>
          )
        })() : undefined
        return (
          <Scoreboard
            redLabel={server.server_vars?.g_redteam ?? 'Red'}
            redScore={server.team_scores.red}
            blueLabel={server.server_vars?.g_blueteam ?? 'Blue'}
            blueScore={server.team_scores.blue}
            state={classifyScores(server.team_scores.red, server.team_scores.blue)}
            live
            redIndicator={redInd}
            blueIndicator={blueInd}
            centerIndicator={centerInd}
          />
        )
      })()}

      {/* 1v1 duel: portraits + score side-by-side. Other modes use the player table. */}
      {server.game_type === '1v1' && activePlayers.length >= 2 ? (
        <Duelists
          left={duelistFromLivePlayer(activePlayers[0])}
          right={duelistFromLivePlayer(activePlayers[1])}
          state={classifyScores(activePlayers[0]?.score ?? 0, activePlayers[1]?.score ?? 0)}
          live
          onPlayerClick={onPlayerClick}
        />
      ) : (
        <PlayerRows
          players={activePlayers.map(p => ({
            name: p.name,
            cleanName: p.clean_name,
            model: p.model,
            team: p.team as 1 | 2 | undefined,
            isBot: p.is_bot,
            skill: p.skill,
            isVR: p.is_vr,
            isVerified: p.is_verified,
            isAdmin: p.is_admin,
            score: p.score,
            ping: p.ping,
            awards: awardsFromCounts(p),
            playerId: p.player_id,
            flagCarrier: flagCarrierForClient(server.flag_status, p.client_num),
          }))}
          mode="live"
          onPlayerClick={onPlayerClick}
        />
      )}

      <SpectatorStrip
        spectators={spectators.map(p => ({ name: p.name }))}
        isLive={true}
      />
    </div>
  )
}

// DuelistData from a live-server Player. Sub-text is ping (live analog of F/D).
function duelistFromLivePlayer(p: Player | undefined): DuelistData {
  if (!p) {
    return { name: '—', score: 0 }
  }
  return {
    name: p.name,
    cleanName: p.clean_name,
    model: p.model,
    isBot: p.is_bot,
    skill: p.skill,
    isVR: p.is_vr,
    isVerified: p.is_verified,
    isAdmin: p.is_admin,
    score: p.score ?? 0,
    sub: <span>{p.ping ?? 0} ping</span>,
    awards: awardsFromCounts(p),
    playerId: p.player_id,
  }
}

// Maps client_num → carried flag (if any).
function flagCarrierForClient(
  flag: import('../types').FlagStatus | undefined,
  clientNum: number,
): 'red' | 'blue' | 'neutral' | undefined {
  if (!flag) return undefined
  if (flag.mode === 'ctf') {
    if (flag.red_carrier === clientNum && flag.red === 1) return 'red'
    if (flag.blue_carrier === clientNum && flag.blue === 1) return 'blue'
  }
  if (flag.mode === '1fctf' && flag.neutral_carrier === clientNum) {
    return 'neutral'
  }
  return undefined
}
