import { useState } from 'react'
import { useLocation } from 'react-router-dom'
import { Link } from 'react-router-dom'
import type { MatchSummary, MatchPlayerSummary } from '../types'
import { ModeIcons } from './ServerCard'
import { RichChip } from './cards/RichChip'
import { Scoreboard } from './cards/Scoreboard'
import { Duelists, type DuelistData } from './cards/Duelists'
import { PlayerRows } from './cards/PlayerRows'
import { SpectatorStrip } from './cards/SpectatorStrip'
import { classifyScores, awardsFromCounts } from './cards/format'
import { PlayIcon } from './PlayIcon'
import { StarIcon } from './StarIcon'
import { useMapMeta } from '../hooks/useMapMeta'
import { useSources } from '../hooks/useSources'
import { useAuth } from '../hooks/useAuth'

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

// Sort active players. Team modes group by team (red, blue, others)
// then by score within. Non-team keeps API order (already score desc).
// In every section, players who didn't finish (completed === false)
// float to the bottom — interleaving them with finishers reads as a
// scoreboard glitch. Array.sort is stable in ES2019+, so connected
// players retain their incoming order.
function sortMatchPlayers(players: MatchPlayerSummary[], isTeam: boolean): MatchPlayerSummary[] {
  const dropOrder = (p: MatchPlayerSummary) => (p.completed === false ? 1 : 0)
  if (!isTeam) {
    return [...players].sort((a, b) => dropOrder(a) - dropOrder(b))
  }
  const teamOrder = (t?: number) => (t === 1 ? 0 : t === 2 ? 1 : 2)
  const score = (p: MatchPlayerSummary) => p.score ?? p.frags ?? 0
  return [...players].sort((a, b) => {
    const td = teamOrder(a.team) - teamOrder(b.team)
    if (td !== 0) return td
    const dd = dropOrder(a) - dropOrder(b)
    if (dd !== 0) return dd
    return score(b) - score(a)
  })
}

function demoFilename(match: MatchSummary): string {
  const d = new Date(match.started_at)
  const pad = (n: number) => String(n).padStart(2, '0')
  const date = `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}`
  const time = `${pad(d.getHours())}${pad(d.getMinutes())}${pad(d.getSeconds())}`
  return `${date}_${time}_${match.map_name}.tvd`
}

// DuelistData from a finished match. Awards come from per-match counters.
function duelistFromMatchPlayer(p: MatchPlayerSummary | undefined): DuelistData {
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
    score: p.frags ?? 0,
    sub: <span>{p.frags ?? 0} K · {p.deaths ?? 0} D</span>,
    awards: awardsFromCounts(p),
    playerId: p.player_id,
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
  onPlayerClick,
  // TODO: wire highlightPlayerId / showPermalink through the card primitives.
}: MatchCardProps) {
  const { hasMultiple: hasMultipleSources } = useSources()
  const meta = useMapMeta(match.map_name)
  const location = useLocation()
  const { auth } = useAuth()

  // Optimistic featured state — admins can toggle from the card without
  // navigating to the detail page. Resyncs to the prop when the match
  // changes (new match, or parent refetch with updated value) via
  // during-render adjustment so there's no extra commit cycle.
  const [isFeatured, setIsFeatured] = useState(match.is_featured ?? false)
  const [featurePending, setFeaturePending] = useState(false)
  const [prevSync, setPrevSync] = useState({ id: match.id, value: match.is_featured })
  if (prevSync.id !== match.id || prevSync.value !== match.is_featured) {
    setPrevSync({ id: match.id, value: match.is_featured })
    setIsFeatured(match.is_featured ?? false)
  }

  const canFeature = auth.isAdmin && !!match.demo_url
  const toggleFeatured = async (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (!auth.token || featurePending) return
    const next = !isFeatured
    setFeaturePending(true)
    setIsFeatured(next)
    try {
      const res = await fetch(`/api/admin/matches/${match.id}/feature`, {
        method: next ? 'POST' : 'DELETE',
        headers: { Authorization: `Bearer ${auth.token}` },
      })
      if (!res.ok) setIsFeatured(!next)
    } catch {
      setIsFeatured(!next)
    } finally {
      setFeaturePending(false)
    }
  }
  const isTeam = isTeamGame(match.game_type)
  const isDuel = match.game_type === '1v1'
  const players = match.players ?? []
  const activeTeams = sortMatchPlayers(
    players.filter((p) => !isSpectator(p)),
    isTeam,
  )
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
        <PlayIcon size={10} />
        Watch
      </Link>
      <a href={match.demo_url} download={demoFilename(match)} className="demo-action download">
        Download
      </a>
    </span>
  ) : null

  const rowData = activeTeams.map((p) => ({
    name: p.name,
    cleanName: p.clean_name,
    model: p.model,
    team: p.team as 1 | 2 | 3 | undefined,
    isBot: p.is_bot,
    skill: p.skill,
    isVR: p.is_vr,
    isVerified: p.is_verified,
    isAdmin: p.is_admin,
    frags: p.frags,
    deaths: p.deaths,
    score: p.score,
    awards: awardsFromCounts(p),
    playerId: p.player_id,
    completed: p.completed,
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
          server={match.server_key}
          mode={formatGameType(match.game_type)}
        >
          <ModeIcons movement={match.movement} gameplay={match.gameplay} />
        </RichChip>
        {canFeature && (
          <button
            type="button"
            className={`card__feature-toggle${isFeatured ? ' is-featured' : ''}`}
            onClick={toggleFeatured}
            disabled={featurePending}
            title={isFeatured ? 'Unfeature this demo' : 'Feature this demo'}
            aria-label={isFeatured ? 'Unfeature this demo' : 'Feature this demo'}
            aria-pressed={isFeatured}
          >
            <StarIcon filled={isFeatured} size={16} />
          </button>
        )}
        {match.ended_at && (
          <Link
            to={`/matches/${match.id}`}
            state={{ from: location.pathname }}
            className="card__time"
            title={`${new Date(match.ended_at).toLocaleString()} — open match details`}
          >
            {formatTimeAgo(match.ended_at)}
            <span className="duration">{formatDuration(match.started_at, match.ended_at)}</span>
          </Link>
        )}
      </div>

      <div className="card__header">
        <div className="card__map">
          {meta.longName && <span className="card__map-short">{meta.shortName}</span>}
          <span className="card__map-long">{meta.displayName}</span>
        </div>
      </div>

      {isDuel && activeTeams.length >= 2 ? (
        <Duelists
          left={duelistFromMatchPlayer(activeTeams[0])}
          right={duelistFromMatchPlayer(activeTeams[1])}
          state={classifyScores(activeTeams[0]?.frags ?? 0, activeTeams[1]?.frags ?? 0)}
          onPlayerClick={onPlayerClick}
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
          <PlayerRows players={rowData} mode="finished" onPlayerClick={onPlayerClick} />
        </>
      )}

      <SpectatorStrip spectators={spectators} isLive={false} rightSlot={demoActions} />
    </article>
  )
}
