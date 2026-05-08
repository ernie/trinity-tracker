import { useState, useEffect } from 'react'
import { Link, useParams, useNavigate } from 'react-router-dom'
import { MatchCard } from './MatchCard'
import { Header } from './Header'
import { Breadcrumbs } from './Breadcrumbs'
import type { MatchSummary } from '../types'

export function MatchDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [match, setMatch] = useState<MatchSummary | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    async function fetchMatch() {
      if (!id) return

      try {
        setLoading(true)
        setError(null)
        const res = await fetch(`/api/matches/${id}`)
        if (res.status === 404) {
          setError('Match not found')
          return
        }
        if (!res.ok) {
          setError('Failed to load match')
          return
        }
        const data = await res.json()
        setMatch(data)
      } catch (e) {
        console.error('Failed to fetch match:', e)
        setError('Failed to load match')
      } finally {
        setLoading(false)
      }
    }

    fetchMatch()
  }, [id])

  const handlePlayerClick = (_playerName: string, _cleanName: string, playerId?: number) => {
    if (playerId) {
      navigate(`/players/${playerId}`)
    }
  }

  return (
    <div className="match-detail-page">
      <Header title="Match Details" className="match-detail-header" />

      {match && (
        <Breadcrumbs
          crumbs={[
            { label: 'Matches', to: '/matches' },
            {
              label: (() => {
                const date = match.started_at ? new Date(match.started_at).toLocaleDateString() : ''
                const map = match.map_name ?? ''
                return [date, map].filter(Boolean).join(' · ') || 'Match'
              })(),
            },
          ]}
        />
      )}

      <div className="match-detail-content">
        <div className="match-detail-nav">
          <Link to="/matches" className="back-link">&larr; Back to Matches</Link>
        </div>

        {loading ? (
          <div className="stats-loading">Loading match...</div>
        ) : error ? (
          <div className="stats-error">{error}</div>
        ) : match ? (
          <div className="match-detail-card-container">
            <MatchCard
              match={match}
              onPlayerClick={handlePlayerClick}
            />
          </div>
        ) : null}
      </div>
    </div>
  )
}
