import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ColoredText } from '../ColoredText'
import { PlayerPortrait } from '../PlayerPortrait'
import { ArrowIcon } from '../ArrowIcon'
import type { LeaderboardEntry, LeaderboardResponse } from '../../types'

export function TopPlayersStrip() {
  const [entries, setEntries] = useState<LeaderboardEntry[]>([])
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    let cancelled = false
    fetch('/api/stats/leaderboard?category=victories&period=week&limit=10')
      .then((r) => (r.ok ? r.json() : { entries: [] }))
      .catch(() => ({ entries: [] }))
      .then((data: LeaderboardResponse | { entries: LeaderboardEntry[] }) => {
        if (cancelled) return
        setEntries(data.entries ?? [])
        setLoaded(true)
      })
    return () => { cancelled = true }
  }, [])

  if (loaded && entries.length === 0) return null

  return (
    <section className="landing-section">
      <header className="landing-section__head">
        <h2 className="landing-section__title">SHARPEST THIS WEEK.</h2>
        <Link to="/leaderboard" className="landing-section__cta">Full leaderboard <ArrowIcon direction="right" /></Link>
      </header>
      <div className="landing-shelf-h">
        <div className="landing-shelf-h__scroll landing-players-strip">
          {entries.map((p) => {
            const isTop = p.rank <= 3
            const cleanInitial = (p.player.clean_name || p.player.name || '').replace(/\^./g, '').charAt(0).toLowerCase() || '?'
            return (
              <Link
                key={p.player.id}
                to={`/players/${p.player.id}`}
                className={`landing-player-card ${isTop ? 'top' : ''}`}
              >
                <span className={`landing-player-card__rank ${isTop ? 'top' : ''}`}>#{p.rank}</span>
                <span className="landing-player-card__avatar">
                  <PlayerPortrait model={p.player.model} size="lg" fallback={cleanInitial} />
                </span>
                <span className="landing-player-card__name">
                  <ColoredText text={p.player.name} />
                </span>
                <span className="landing-player-card__stat">{p.victories}</span>
                <span className="landing-player-card__stat-label">victories</span>
              </Link>
            )
          })}
        </div>
      </div>
    </section>
  )
}
