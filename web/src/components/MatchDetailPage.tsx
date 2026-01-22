import { useState, useEffect } from 'react'
import { Link, useParams, useNavigate } from 'react-router-dom'
import { AppLogo } from './AppLogo'
import { PageNav } from './PageNav'
import { MatchCard } from './MatchCard'
import { LoginForm } from './LoginForm'
import { useAuth } from '../hooks/useAuth'
import type { MatchSummary } from '../types'

export function MatchDetailPage() {
  const { auth, login, logout } = useAuth()
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
      <header className="match-detail-header">
        <h1>
          <AppLogo />
          Match Details
        </h1>
        <PageNav />
        <div className="auth-section">
          {auth.isAuthenticated ? (
            <div className="user-info">
              <Link to="/account" className="username-link">{auth.username}</Link>
              <button onClick={logout} className="logout-btn">Logout</button>
            </div>
          ) : (
            <LoginForm onLogin={(username, password) => login({ username, password })} />
          )}
        </div>
      </header>

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
