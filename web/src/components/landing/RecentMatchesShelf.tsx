import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { formatDuration } from '../MatchCard'
import { PlayIcon } from '../PlayIcon'
import { useMapMeta } from '../../hooks/useMapMeta'
import type { MatchSummary } from '../../types'

export function RecentMatchesShelf() {
  const navigate = useNavigate()
  const [matches, setMatches] = useState<MatchSummary[]>([])

  useEffect(() => {
    let cancelled = false
    fetch('/api/matches?limit=20')
      .then((res) => (res.ok ? res.json() : []))
      .then((data: MatchSummary[]) => {
        if (cancelled) return
        // Take the 5 most-recent demo-available matches.
        const filtered = (data ?? [])
          .filter((m) => !!m.demo_url)
          .slice(0, 5)
        setMatches(filtered)
      })
      .catch(() => { if (!cancelled) setMatches([]) })
    return () => { cancelled = true }
  }, [])

  if (matches.length === 0) return null

  return (
    <section className="landing-section">
      <header className="landing-section__head">
        <h2 className="landing-section__title">THE LAST FEW FIGHTS.</h2>
        <Link to="/matches" className="landing-section__cta">All matches →</Link>
      </header>
      <ul className="landing-matches">
        {matches.map((m) => (
          <li key={m.id}>
            <a
              className="landing-match-row"
              href={`/matches/${m.id}/demo`}
              onClick={(e) => {
                e.preventDefault()
                navigate(`/matches/${m.id}/demo`, { state: { from: '/' } })
              }}
            >
              <ModeChip movement={m.movement} gameType={m.game_type} />
              <MatchTitle mapName={m.map_name} />
              <Score red={m.red_score} blue={m.blue_score} />
              <span className="landing-match-row__meta">
                <time>{relativeTime(m.ended_at)}</time>
                <span className="duration">
                  {m.started_at && m.ended_at ? formatDuration(m.started_at, m.ended_at) : ''}
                </span>
              </span>
              <span className="landing-match-row__demo">
                <PlayIcon size={9} />
                Watch
              </span>
            </a>
          </li>
        ))}
      </ul>
    </section>
  )
}

function ModeChip({ movement, gameType }: { movement?: string; gameType?: string }) {
  const label = [moveLabel(movement), gameTypeLabel(gameType)].filter(Boolean).join(' · ')
  if (!label) return <span className="landing-match-row__mode" />
  return <span className="landing-match-row__mode">{label}</span>
}

function moveLabel(movement?: string) {
  switch (movement) {
    case '0': return 'VQ3'
    case '1': return 'CPM'
    case '2': return 'QL'
    case '3': return 'QLT'
    default: return ''
  }
}

function gameTypeLabel(t?: string) {
  return (t ?? '').toUpperCase()
}

function MatchTitle({ mapName }: { mapName: string }) {
  const meta = useMapMeta(mapName)
  return (
    <div className="landing-match-row__title">
      {meta.longName && <span className="landing-match-row__map-short">{meta.shortName}</span>}
      <span className="landing-match-row__map-long">{meta.displayName}</span>
    </div>
  )
}

function Score({ red, blue }: { red?: number | null; blue?: number | null }) {
  if (red == null && blue == null) {
    return <span className="landing-match-row__score landing-match-row__score--empty" aria-hidden />
  }
  return (
    <span className="landing-match-row__score">
      {red ?? 0}<span className="sep">–</span>{blue ?? 0}
    </span>
  )
}

// Returns "12m ago" / "3h ago" / "2d ago" / formatted date for older.
function relativeTime(endedAt?: string): string {
  if (!endedAt) return ''
  const ts = new Date(endedAt).getTime()
  if (Number.isNaN(ts)) return ''
  const diff = Date.now() - ts
  const minutes = Math.floor(diff / 60_000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 7) return `${days}d ago`
  return new Date(endedAt).toLocaleDateString()
}
