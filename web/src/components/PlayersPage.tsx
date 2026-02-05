import { useState, useCallback } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { AppLogo } from './AppLogo'
import { BotBadge } from './BotBadge'
import { PageNav } from './PageNav'
import { ColoredText } from './ColoredText'
import { PlayerPortrait } from './PlayerPortrait'
import { PlayerRecentMatches } from './PlayerRecentMatches'
import { PlayerSessions } from './PlayerSessions'
import { PlayerBadge } from './PlayerBadge'
import { LoginForm } from './LoginForm'
import { UserManagement } from './UserManagement'
import { StatItem } from './StatItem'
import { PeriodSelector } from './PeriodSelector'
import { useAuth } from '../hooks/useAuth'
import { usePlayerStats } from '../hooks/usePlayerStats'
import { formatDate, formatDuration } from '../utils/formatters'
import { stripVRPrefix } from '../utils'
import type { TimePeriod, PlayerProfile, PlayerGUID } from '../types'

export function PlayersPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { auth, login, logout } = useAuth()

  const [searchQuery, setSearchQuery] = useState('')
  const [searchResults, setSearchResults] = useState<PlayerProfile[]>([])
  const [searching, setSearching] = useState(false)

  const [period, setPeriod] = useState<TimePeriod>('all')
  const { stats, loading, error, refetch } = usePlayerStats(id ? Number(id) : undefined, period)

  // Admin state
  const [showUserManagement, setShowUserManagement] = useState(false)
  const [showMergeSearch, setShowMergeSearch] = useState(false)
  const [mergeQuery, setMergeQuery] = useState('')
  const [mergeResults, setMergeResults] = useState<PlayerProfile[]>([])
  const [mergeSearching, setMergeSearching] = useState(false)
  const [merging, setMerging] = useState(false)
  const [splitting, setSplitting] = useState<number | null>(null)

  // Search for players (includes GUID search if admin)
  const handleSearch = useCallback(() => {
    if (!searchQuery.trim()) {
      setSearchResults([])
      return
    }

    const headers: HeadersInit = {}
    if (auth.token) {
      headers['Authorization'] = `Bearer ${auth.token}`
    }

    setSearching(true)
    fetch(`/api/players?search=${encodeURIComponent(searchQuery)}&limit=10`, { headers })
      .then(res => res.json())
      .then(data => setSearchResults(data || []))
      .catch(() => setSearchResults([]))
      .finally(() => setSearching(false))
  }, [searchQuery, auth.token])

  // Search on Enter key
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      handleSearch()
    }
  }

  // Select a player from search results
  const selectPlayer = (playerId: number) => {
    navigate(`/players/${playerId}`)
    setSearchResults([])
    setSearchQuery('')
  }

  // Search for players to merge (includes GUID search since admin)
  const handleMergeSearch = useCallback(() => {
    if (!mergeQuery.trim()) {
      setMergeResults([])
      return
    }

    const headers: HeadersInit = {}
    if (auth.token) {
      headers['Authorization'] = `Bearer ${auth.token}`
    }

    setMergeSearching(true)
    fetch(`/api/players?search=${encodeURIComponent(mergeQuery)}&limit=10`, { headers })
      .then(res => res.json())
      .then(data => {
        // Filter out the current player from results
        const filtered = (data || []).filter((p: PlayerProfile) => p.id !== Number(id))
        setMergeResults(filtered)
      })
      .catch(() => setMergeResults([]))
      .finally(() => setMergeSearching(false))
  }, [mergeQuery, id, auth.token])

  // Merge another player into this one
  const handleMerge = async (mergePlayerId: number) => {
    if (!auth.token || !id) return

    if (!confirm('Are you sure you want to merge this player? This will move all their GUIDs and stats to the current player.')) {
      return
    }

    setMerging(true)
    try {
      const res = await fetch(`/api/admin/players/${id}/merge`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${auth.token}`,
        },
        body: JSON.stringify({ merge_player_id: mergePlayerId }),
      })

      if (!res.ok) {
        const data = await res.json()
        throw new Error(data.error || 'Merge failed')
      }

      // Refresh player data
      setShowMergeSearch(false)
      setMergeQuery('')
      setMergeResults([])
      refetch()
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Merge failed')
    } finally {
      setMerging(false)
    }
  }

  // Split a GUID into a new player
  const handleSplit = async (guidId: number) => {
    if (!auth.token) return

    if (!confirm('Are you sure you want to split this GUID into a separate player?')) {
      return
    }

    setSplitting(guidId)
    try {
      const res = await fetch(`/api/admin/guids/${guidId}/split`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${auth.token}`,
        },
      })

      if (!res.ok) {
        const data = await res.json()
        throw new Error(data.error || 'Split failed')
      }

      const newPlayer = await res.json()
      // Navigate to the new player
      navigate(`/players/${newPlayer.id}`)
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Split failed')
    } finally {
      setSplitting(null)
    }
  }

  return (
    <div className="players-page">
      <header className="players-header">
        <h1>
          <AppLogo />
          Player Stats
        </h1>
        <PageNav />
        <div className="auth-section">
          {auth.isAuthenticated ? (
            <div className="user-info">
              <Link to="/account" className="username-link">{auth.username}</Link>
              {auth.isAdmin && (
                <button onClick={() => setShowUserManagement(true)} className="admin-btn">Users</button>
              )}
              <button onClick={logout} className="logout-btn">Logout</button>
            </div>
          ) : (
            <LoginForm onLogin={(username, password) => login({ username, password })} />
          )}
        </div>
      </header>

      <div className="players-search">
        <input
          type="text"
          placeholder={auth.isAuthenticated ? "Search players by name or GUID..." : "Search players by name..."}
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          className="search-input"
        />
        <button onClick={handleSearch} disabled={searching} className="search-btn">
          {searching ? 'Searching...' : 'Search'}
        </button>
      </div>

      {searchResults.length > 0 && (
        <div className="search-results">
          {searchResults.map(player => (
            <div
              key={player.id}
              className="search-result-item"
              onClick={() => selectPlayer(player.id)}
            >
              <span className="player-name-with-badges">
                {player.is_bot && <BotBadge isBot skill={5} />}
                {!player.is_bot && <PlayerBadge playerId={player.id} isVR={player.is_vr} />}
                <ColoredText text={player.is_vr ? stripVRPrefix(player.name) : player.name} />
              </span>
              <span className="player-last-seen">Last seen: {formatDate(player.last_seen)}</span>
            </div>
          ))}
        </div>
      )}

      {id && (
        <div className="player-stats-container">
          <PeriodSelector period={period} onChange={setPeriod} />

          {loading ? (
            <div className="stats-loading">Loading stats...</div>
          ) : error ? (
            <div className="stats-error">{error}</div>
          ) : stats ? (
            <div className="player-stats-full">
              <h2>
                <PlayerPortrait model={stats.player.model} size="lg" />
                {stats.player.is_bot && <BotBadge isBot skill={5} size="lg" />}
                {!stats.player.is_bot && <PlayerBadge playerId={stats.player.id} isVR={stats.player.is_vr} size="lg" />}
                <ColoredText text={stats.player.is_vr ? stripVRPrefix(stats.player.name) : stats.player.name} />
              </h2>

              <div className="player-meta-top">
                <span><em>Seen:</em> {formatDate(stats.player.first_seen)} – {formatDate(stats.player.last_seen)}</span>
                {stats.player.total_playtime_seconds > 0 && (
                  <span><em>Played:</em> {formatDuration(stats.player.total_playtime_seconds)}</span>
                )}
              </div>

              <div className="stats-grid">
                <StatItem
                  label="Matches"
                  value={stats.stats.completed_matches}
                  subscript={stats.stats.uncompleted_matches > 0 ? stats.stats.uncompleted_matches : undefined}
                  title={stats.stats.uncompleted_matches > 0
                    ? `${stats.stats.completed_matches} completed, ${stats.stats.uncompleted_matches} incomplete`
                    : undefined}
                />
                <StatItem label="K/D" value={stats.stats.kd_ratio.toFixed(2)} />
                <StatItem label="Frags" value={stats.stats.frags} className="frags" />
                <StatItem label="Deaths" value={stats.stats.deaths} className="deaths" />
                <StatItem label="Victories" value={stats.stats.victories} />
                <StatItem label="Excellent" value={stats.stats.excellents} />
                <StatItem label="Impressive" value={stats.stats.impressives} />
                <StatItem label="Humiliation" value={stats.stats.humiliations} />
                <StatItem label="Captures" value={stats.stats.captures} />
                <StatItem label="Returns" value={stats.stats.flag_returns} />
                <StatItem label="Assists" value={stats.stats.assists} />
                <StatItem label="Defense" value={stats.stats.defends} />
              </div>

              {stats.names && (() => {
                const uniqueNames = [...new Set(stats.names.map(n => n.name))].filter(name => name !== stats.player.name)
                return uniqueNames.length > 0 && (
                  <div className="also-known-as">
                    <h4>Also known as</h4>
                    <div className="name-list">
                      {uniqueNames.slice(0, 9).map((name, i) => (
                        <span key={i} className="aka-name">
                          <ColoredText text={name} />
                        </span>
                      ))}
                    </div>
                  </div>
                )
              })()}

              {/* Admin: GUIDs section */}
              {auth.isAuthenticated && stats.player.guids && stats.player.guids.length > 0 && (
                <div className="player-guids-section">
                  <h4>Linked Accounts ({stats.player.guids.length})</h4>
                  <div className="guids-list">
                    {stats.player.guids.map((guid: PlayerGUID) => (
                      <div key={guid.id} className="guid-item">
                        <div className="guid-info">
                          <ColoredText text={guid.name} />
                          <span className="guid-hash">{guid.guid}</span>
                          <span className="guid-dates">
                            {formatDate(guid.first_seen)} - {formatDate(guid.last_seen)}
                          </span>
                        </div>
                        {stats.player.guids && stats.player.guids.length > 1 && (
                          <button
                            className="split-btn"
                            onClick={() => handleSplit(guid.id)}
                            disabled={splitting === guid.id}
                            title="Split this account into a separate player"
                          >
                            {splitting === guid.id ? 'Splitting...' : 'Split'}
                          </button>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Admin: Sessions section */}
              {auth.isAdmin && auth.token && (
                <PlayerSessions playerId={stats.player.id} token={auth.token} />
              )}

              {/* Admin: Merge controls */}
              {auth.isAuthenticated && (
                <div className="admin-controls">
                  {!showMergeSearch ? (
                    <button
                      className="merge-toggle-btn"
                      onClick={() => setShowMergeSearch(true)}
                    >
                      Merge Another Player
                    </button>
                  ) : (
                    <div className="merge-search-panel">
                      <div className="merge-search-header">
                        <h4>Merge Player Into This One</h4>
                        <button
                          className="close-btn"
                          onClick={() => {
                            setShowMergeSearch(false)
                            setMergeQuery('')
                            setMergeResults([])
                          }}
                        >
                          Cancel
                        </button>
                      </div>
                      <div className="merge-search-input">
                        <input
                          type="text"
                          placeholder="Search players by name or GUID..."
                          value={mergeQuery}
                          onChange={(e) => setMergeQuery(e.target.value)}
                          onKeyDown={(e) => e.key === 'Enter' && handleMergeSearch()}
                        />
                        <button onClick={handleMergeSearch} disabled={mergeSearching}>
                          {mergeSearching ? 'Searching...' : 'Search'}
                        </button>
                      </div>
                      {mergeResults.length > 0 && (
                        <div className="merge-results">
                          {mergeResults.map(player => (
                            <div key={player.id} className="merge-result-item">
                              <div className="merge-player-info">
                                <ColoredText text={player.name} />
                                <span className="merge-player-date">
                                  Last seen: {formatDate(player.last_seen)}
                                </span>
                              </div>
                              <button
                                className="merge-btn"
                                onClick={() => handleMerge(player.id)}
                                disabled={merging}
                              >
                                {merging ? 'Merging...' : 'Merge'}
                              </button>
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )}

              <PlayerRecentMatches playerId={stats.player.id} />
            </div>
          ) : null}
        </div>
      )}

      {!id && !searchResults.length && (
        <div className="players-empty">
          <p>Search for a player by name to view their statistics.</p>
        </div>
      )}

      {showUserManagement && auth.isAdmin && auth.token && (
        <UserManagement
          token={auth.token}
          currentUsername={auth.username!}
          onClose={() => setShowUserManagement(false)}
        />
      )}
    </div>
  )
}
