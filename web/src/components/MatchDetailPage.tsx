import { useState, useEffect, useCallback } from 'react'
import { Link, useParams, useNavigate } from 'react-router-dom'
import { MatchCard } from './MatchCard'
import { Header } from './Header'
import { Breadcrumbs } from './Breadcrumbs'
import { useAuth } from '../hooks/useAuth'
import type { MatchSummary } from '../types'

export function MatchDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { auth } = useAuth()
  const [match, setMatch] = useState<MatchSummary | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [featurePending, setFeaturePending] = useState(false)

  const fetchMatch = useCallback(async () => {
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
  }, [id])

  useEffect(() => {
    fetchMatch()
  }, [fetchMatch])

  const toggleFeatured = async () => {
    if (!match || !auth.token) return
    const next = !match.is_featured
    setFeaturePending(true)
    try {
      const res = await fetch(`/api/admin/matches/${match.id}/feature`, {
        method: next ? 'POST' : 'DELETE',
        headers: { Authorization: `Bearer ${auth.token}` },
      })
      if (!res.ok) {
        console.error('Feature toggle failed:', res.status)
        return
      }
      // Optimistic update + refetch to confirm server state.
      setMatch({ ...match, is_featured: next })
      fetchMatch()
    } finally {
      setFeaturePending(false)
    }
  }

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
          <>
            {auth.isAdmin && (
              <div className="match-feature-row">
                <button
                  type="button"
                  className={`match-feature-toggle${match.is_featured ? ' is-featured' : ''}`}
                  onClick={toggleFeatured}
                  disabled={featurePending}
                  title="Featured matches surface on the landing page's Watch-a-fight door"
                >
                  {match.is_featured ? '★ Featured' : '☆ Feature this demo'}
                </button>
              </div>
            )}
            <div className="match-detail-card-container">
              <MatchCard
                match={match}
                onPlayerClick={handlePlayerClick}
              />
            </div>
          </>
        ) : null}
      </div>
    </div>
  )
}
