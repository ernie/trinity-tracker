import { useState, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { BotBadge } from './BotBadge'
import { ColoredText } from './ColoredText'
import { PlayerPortrait } from './PlayerPortrait'
import { PlayerRecentMatches } from './PlayerRecentMatches'
import { PlayerSessions } from './PlayerSessions'
import { PlayerBadge } from './PlayerBadge'
import { Header } from './Header'
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
  const { auth } = useAuth()

  const [searchQuery, setSearchQuery] = useState('')
  const [searchResults, setSearchResults] = useState<PlayerProfile[]>([])
  const [searching, setSearching] = useState(false)

  const [period, setPeriod] = useState<TimePeriod>('all')
  const { stats, loading, error } = usePlayerStats(id ? Number(id) : undefined, period)

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

  return (
    <div className="players-page">
      <Header title="Player Stats" className="players-header" />

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
                {!player.is_bot && <PlayerBadge isVerified={player.is_verified} isAdmin={player.is_admin} isVR={player.is_vr} />}
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
                {!stats.player.is_bot && <PlayerBadge isVerified={stats.player.is_verified} isAdmin={stats.player.is_admin} isVR={stats.player.is_vr} size="lg" />}
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
                  title={stats.stats.uncompleted_matches > 0
                    ? `${stats.stats.completed_matches} completed, ${stats.stats.uncompleted_matches} incomplete`
                    : undefined}
                />
                <StatItem label="K/D" value={stats.stats.kd_ratio.toFixed(2)} />
                <StatItem label="Frags" value={stats.stats.frags} className="frags" />
                <StatItem label="Deaths" value={stats.stats.deaths} className="deaths" />
                <StatItem label="Victories" value={stats.stats.victories} backgroundIcon="/assets/medals/medal_victory.png" />
                <StatItem label="Excellent" value={stats.stats.excellents} backgroundIcon="/assets/medals/medal_excellent.png" />
                <StatItem label="Impressive" value={stats.stats.impressives} backgroundIcon="/assets/medals/medal_impressive.png" />
                <StatItem label="Humiliation" value={stats.stats.humiliations} backgroundIcon="/assets/medals/medal_gauntlet.png" />
                <StatItem label="Captures" value={stats.stats.captures} backgroundIcon="/assets/medals/medal_capture.png" />
                <StatItem label="Returns" value={stats.stats.flag_returns} backgroundIcon="/assets/flags/flag_in_base_red.png" />
                <StatItem label="Assists" value={stats.stats.assists} backgroundIcon="/assets/medals/medal_assist.png" />
                <StatItem label="Defense" value={stats.stats.defends} backgroundIcon="/assets/medals/medal_defend.png" />
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

              {/* Linked GUIDs (informational) */}
              {auth.isAuthenticated && stats.player.guids && stats.player.guids.length > 0 && (
                <div className="player-guids-section">
                  <h4>Linked GUIDs ({stats.player.guids.length})</h4>
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
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Admin: Sessions section (hidden for bots) */}
              {auth.isAdmin && auth.token && !stats.player.is_bot && (
                <PlayerSessions playerId={stats.player.id} token={auth.token} />
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

    </div>
  )
}
