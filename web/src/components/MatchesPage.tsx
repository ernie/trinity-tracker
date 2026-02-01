import { useState, useEffect } from 'react'
import { Link, useSearchParams, useNavigate } from 'react-router-dom'
import { AppLogo } from './AppLogo'
import { PageNav } from './PageNav'
import { MatchCard } from './MatchCard'
import { LoginForm } from './LoginForm'
import { useAuth } from '../hooks/useAuth'
import { GAME_TYPE_LABELS, type GameTypeFilter } from '../constants/labels'
import type { MatchSummary } from '../types'

const PAGE_SIZE = 10

function parseDateTimeLocal(value: string): Date | null {
  if (!value) return null
  const date = new Date(value)
  return isNaN(date.getTime()) ? null : date
}

export function MatchesPage() {
  const { auth, login, logout } = useAuth()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()

  // Read initial state from URL
  const [gameType, setGameType] = useState<GameTypeFilter>(() => {
    const gt = searchParams.get('game_type')
    return (gt && gt in GAME_TYPE_LABELS) ? gt as GameTypeFilter : 'all'
  })
  const [startDate, setStartDate] = useState<string>(() => searchParams.get('start_date') || '')
  const [endDate, setEndDate] = useState<string>(() => searchParams.get('end_date') || '')
  const [includeBotOnly, setIncludeBotOnly] = useState<boolean>(() =>
    searchParams.get('include_bot_only') === 'true'
  )

  const [matches, setMatches] = useState<MatchSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [hasMore, setHasMore] = useState(true)

  // Update URL when filters change
  useEffect(() => {
    const params = new URLSearchParams()
    if (gameType !== 'all') params.set('game_type', gameType)
    if (startDate) params.set('start_date', startDate)
    if (endDate) params.set('end_date', endDate)
    if (includeBotOnly) params.set('include_bot_only', 'true')
    setSearchParams(params, { replace: true })
  }, [gameType, startDate, endDate, includeBotOnly, setSearchParams])

  // Fetch matches when filters change
  useEffect(() => {
    async function fetchMatches() {
      try {
        setLoading(true)
        setMatches([])
        setHasMore(true)

        const params = new URLSearchParams()
        params.set('limit', PAGE_SIZE.toString())
        if (gameType !== 'all') params.set('game_type', gameType)
        if (includeBotOnly) params.set('include_bot_only', 'true')

        // Convert datetime-local to RFC3339
        if (startDate) {
          const date = parseDateTimeLocal(startDate)
          if (date) params.set('start_date', date.toISOString())
        }
        if (endDate) {
          const date = parseDateTimeLocal(endDate)
          if (date) params.set('end_date', date.toISOString())
        }

        const res = await fetch(`/api/matches?${params.toString()}`)
        if (res.ok) {
          const data = await res.json()
          const fetchedMatches = data ?? []
          setMatches(fetchedMatches)
          setHasMore(fetchedMatches.length === PAGE_SIZE)
        }
      } catch (e) {
        console.error('Failed to fetch matches:', e)
      } finally {
        setLoading(false)
      }
    }

    fetchMatches()
  }, [gameType, startDate, endDate, includeBotOnly])

  const loadMore = async () => {
    if (loadingMore || !hasMore || matches.length === 0) return

    const lastMatchId = matches[matches.length - 1].id
    try {
      setLoadingMore(true)

      const params = new URLSearchParams()
      params.set('limit', PAGE_SIZE.toString())
      params.set('before', lastMatchId.toString())
      if (gameType !== 'all') params.set('game_type', gameType)
      if (includeBotOnly) params.set('include_bot_only', 'true')
      if (startDate) {
        const date = parseDateTimeLocal(startDate)
        if (date) params.set('start_date', date.toISOString())
      }
      if (endDate) {
        const date = parseDateTimeLocal(endDate)
        if (date) params.set('end_date', date.toISOString())
      }

      const res = await fetch(`/api/matches?${params.toString()}`)
      if (res.ok) {
        const data = await res.json()
        const newMatches = data ?? []
        setMatches(prev => [...prev, ...newMatches])
        setHasMore(newMatches.length === PAGE_SIZE)
      }
    } catch (e) {
      console.error('Failed to fetch more matches:', e)
    } finally {
      setLoadingMore(false)
    }
  }

  const handlePlayerClick = (_playerName: string, _cleanName: string, playerId?: number) => {
    if (playerId) {
      navigate(`/players/${playerId}`)
    }
  }

  const clearFilters = () => {
    setGameType('all')
    setStartDate('')
    setEndDate('')
    setIncludeBotOnly(false)
  }

  const hasActiveFilters = gameType !== 'all' || startDate || endDate || includeBotOnly

  return (
    <div className="matches-page">
      <header className="matches-header">
        <h1>
          <AppLogo />
          Match Browser
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

      <div className="match-filters">
        <div className="game-type-selector">
          {(Object.keys(GAME_TYPE_LABELS) as GameTypeFilter[]).map((gt) => (
            <button
              key={gt}
              className={`game-type-btn ${gameType === gt ? 'active' : ''}`}
              onClick={() => setGameType(gt)}
            >
              {GAME_TYPE_LABELS[gt]}
            </button>
          ))}
        </div>

        <div className="date-range-filters">
          <label className="date-filter">
            <span>From:</span>
            <input
              type="datetime-local"
              value={startDate}
              onChange={(e) => setStartDate(e.target.value)}
            />
          </label>
          <label className="date-filter">
            <span>To:</span>
            <input
              type="datetime-local"
              value={endDate}
              onChange={(e) => setEndDate(e.target.value)}
            />
          </label>
          <label className="include-bots-filter">
            <input
              type="checkbox"
              checked={includeBotOnly}
              onChange={(e) => setIncludeBotOnly(e.target.checked)}
            />
            Include bot-only matches
          </label>
          {hasActiveFilters && (
            <button className="clear-filters-btn" onClick={clearFilters}>
              Clear Filters
            </button>
          )}
        </div>
      </div>

      <div className="matches-content">
        {loading ? (
          <div className="stats-loading">Loading matches...</div>
        ) : matches.length === 0 ? (
          <div className="matches-empty">
            No matches found{hasActiveFilters ? ' for the selected filters' : ''}
          </div>
        ) : (
          <>
            <div className="matches-list matches-browser-list">
              {matches.map((match) => (
                <MatchCard
                  key={match.id}
                  match={match}
                  onPlayerClick={handlePlayerClick}
                  showPermalink
                />
              ))}
            </div>
            {hasMore && (
              <div className="load-more-container">
                <button
                  className="load-more-btn"
                  onClick={loadMore}
                  disabled={loadingMore}
                >
                  {loadingMore ? 'Loading...' : 'Load More'}
                </button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
