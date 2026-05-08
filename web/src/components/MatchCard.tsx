import { useLocation } from 'react-router-dom'
import { Link } from 'react-router-dom'
import type { MatchSummary, MatchPlayerSummary } from '../types'
import { ModeIcons } from './ServerCard'
import { RichChip } from './cards/RichChip'
import { Scoreboard } from './cards/Scoreboard'
import { Duelists, type DuelistData } from './cards/Duelists'
import { PlayerRows } from './cards/PlayerRows'
import { SpectatorStrip } from './cards/SpectatorStrip'
import { classifyScores } from './cards/format'
import { useMapMeta } from '../hooks/useMapMeta'
import { useSources } from '../hooks/useSources'
import { serverDisplay } from '../utils'

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
  if (gt === 'one flag ctf' || gt === '1fctf') return '1FCTF'
  if (gt === 'overload') return 'OVL'
  if (gt === 'harvester') return 'HRV'
  return gameType.slice(0, 3).toUpperCase()
}

export function isTeamGame(gameType: string): boolean {
  if (!gameType) return false
  const gt = gameType.toLowerCase()
  return gt === 'team deathmatch' || gt === 'tdm' ||
         gt === 'capture the flag' || gt === 'ctf' ||
         gt === 'one flag ctf' || gt === '1fctf' ||
         gt === 'overload' || gt === 'harvester'
}

function isSpectator(player: MatchPlayerSummary): boolean {
  return player.team === 3 && player.frags === 0 && player.deaths === 0
}

function demoFilename(match: MatchSummary): string {
  const d = new Date(match.started_at)
  const pad = (n: number) => String(n).padStart(2, '0')
  const date = `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}`
  const time = `${pad(d.getHours())}${pad(d.getMinutes())}${pad(d.getSeconds())}`
  return `${date}_${time}_${match.map_name}.tvd`
}

// Builds DuelistData from a finished-match player. Awards aren't mapped yet —
// MatchPlayerSummary's per-medal counters (impressives/excellents/etc.) need a
// rule to decide which to surface for a 1v1 in particular.
function duelistFromMatchPlayer(p: MatchPlayerSummary | undefined): DuelistData {
  if (!p) {
    return { name: '—', portraitChar: '?', score: 0 }
  }
  const initial = (p.clean_name || p.name || '?').replace(/\^./g, '').charAt(0).toLowerCase() || '?'
  return {
    name: p.name,
    portraitChar: initial,
    score: p.frags ?? 0,
    sub: <span>{p.frags ?? 0} F · {p.deaths ?? 0} D</span>,
  }
}

interface MatchCardProps {
  match: MatchSummary
  onPlayerClick?: (playerName: string, cleanName: string, playerId?: number) => void
  highlightPlayerId?: number
  showPermalink?: boolean
}

export function MatchCard({
  match,
  onPlayerClick: _onPlayerClick,
  highlightPlayerId: _highlightPlayerId,
  showPermalink: _showPermalink = false,
}: MatchCardProps) {
  // TODO(card-conformance): wire onPlayerClick / highlight / permalink through the new primitives
  const { hasMultiple: hasMultipleSources } = useSources()
  const meta = useMapMeta(match.map_name)
  const location = useLocation()
  const isTeam = isTeamGame(match.game_type)
  const isDuel = match.game_type === '1v1'
  const players = match.players ?? []
  const activeTeams = players.filter((p) => !isSpectator(p))
  const spectators = players.filter(isSpectator).map((p) => ({ name: p.name }))

  const redScore = match.red_score ?? 0
  const blueScore = match.blue_score ?? 0
  const scoreState = classifyScores(redScore, blueScore)

  const levelshotUrl = match.map_name
    ? `/assets/levelshots/${match.map_name.toLowerCase()}.jpg`
    : undefined

  const demoActions = match.demo_url ? (
    <span className="demo-actions">
      <Link
        to={`/matches/${match.id}/demo`}
        state={{ from: location.pathname }}
        className="demo-action watch"
      >
        Watch
      </Link>
      <a href={match.demo_url} download={demoFilename(match)} className="demo-action download">
        Download
      </a>
    </span>
  ) : null

  // TODO(card-conformance): map per-player award counts to row awards
  const rowData = activeTeams.map((p) => ({
    name: p.name,
    team: p.team as 1 | 2 | 3 | undefined,
    isBot: p.is_bot,
    frags: p.frags,
    deaths: p.deaths,
    score: p.score,
  }))

  return (
    <article
      className={`card match-card${match.server_active === false ? ' inactive-server' : ''}`}
      style={
        levelshotUrl
          ? ({ ['--levelshot']: `url(${levelshotUrl})` } as React.CSSProperties)
          : undefined
      }
    >
      <div className="card__topbar">
        <RichChip
          source={hasMultipleSources ? match.source : undefined}
          server={serverDisplay(match.source, match.server_key, { hasMultipleSources })}
          mode={formatGameType(match.game_type)}
        />
        {match.ended_at && (
          <span className="card__time" title={new Date(match.ended_at).toLocaleString()}>
            {formatTimeAgo(match.ended_at)}
            <span className="duration">{formatDuration(match.started_at, match.ended_at)}</span>
          </span>
        )}
      </div>

      <div className="card__header">
        <div className="card__map">
          {meta.longName && <span className="card__map-short">{meta.shortName}</span>}
          <span className="card__map-long">{meta.displayName}</span>
        </div>
      </div>

      <ModeIcons movement={match.movement} gameplay={match.gameplay} />

      {isDuel && activeTeams.length >= 2 ? (
        <Duelists
          left={duelistFromMatchPlayer(activeTeams[0])}
          right={duelistFromMatchPlayer(activeTeams[1])}
          state={classifyScores(activeTeams[0]?.frags ?? 0, activeTeams[1]?.frags ?? 0)}
        />
      ) : (
        <>
          {isTeam && (
            <Scoreboard
              redLabel="Red"
              redScore={redScore}
              blueLabel="Blue"
              blueScore={blueScore}
              state={scoreState}
            />
          )}
          <PlayerRows players={rowData} mode="finished" />
        </>
      )}

      <SpectatorStrip spectators={spectators} isLive={false} rightSlot={demoActions} />
    </article>
  )
}
