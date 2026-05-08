import { useState, useEffect, useMemo } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { BotBadge } from './BotBadge'
import { ColoredText } from './ColoredText'
import { PlayerPortrait } from './PlayerPortrait'
import { PlayerRecentMatches } from './PlayerRecentMatches'
import { PlayerSessions } from './PlayerSessions'
import { PlayerBadge } from './PlayerBadge'
import { Breadcrumbs } from './Breadcrumbs'
import { StatItem } from './StatItem'
import { PeriodSelector } from './PeriodSelector'
import { useAuth } from '../hooks/useAuth'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import { usePlayerStats } from '../hooks/usePlayerStats'
import { useLiveData } from '../contexts/LiveDataContext'
import { useSources } from '../hooks/useSources'
import { formatDate, formatDuration } from '../utils/formatters'
import { stripVRPrefix, serverDisplay } from '../utils'
import type { TimePeriod, PlayerProfile, PlayerGUID } from '../types'

export function PlayersPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { auth } = useAuth()
  const { servers } = useLiveData()
  const { hasMultiple: hasMultipleSources } = useSources()

  // Live search (includes GUID search if admin) — fires automatically as the user types.
  const [searchQuery, setSearchQuery] = useState('')
  const [searchResults, setSearchResults] = useState<PlayerProfile[]>([])
  const debouncedSearchQuery = useDebouncedValue(searchQuery, 200)

  const [period, setPeriod] = useState<TimePeriod>('all')
  const { stats, loading, error } = usePlayerStats(id ? Number(id) : undefined, period)

  // Currently-online humans across all live servers, with their host
  // server's display name. Drives the right pane's "playing now" panel
  // shown when there's no active search and no selected player.
  const onlinePlayers = useMemo(() => {
    const list: Array<{
      key: string
      playerId: number
      name: string
      cleanName: string
      model?: string
      isVR: boolean
      isVerified: boolean
      isAdmin: boolean
      score: number
      serverName: string
    }> = []
    for (const status of servers.values()) {
      if (!status.online || !status.players) continue
      const serverName = serverDisplay(status.source, status.key, { hasMultipleSources })
      for (const p of status.players) {
        if (p.is_bot || p.team === 3) continue
        if (!p.player_id) continue // can't navigate without a stable id
        list.push({
          key: `${status.server_id}:${p.client_num}`,
          playerId: p.player_id,
          name: p.name,
          cleanName: p.clean_name,
          model: p.model,
          isVR: p.is_vr ?? false,
          isVerified: p.is_verified ?? false,
          isAdmin: p.is_admin ?? false,
          score: p.score ?? 0,
          serverName,
        })
      }
    }
    // Sort alphabetically by clean name so the panel's order is stable
    // across renders (raw map iteration order is by server id).
    list.sort((a, b) => a.cleanName.localeCompare(b.cleanName))
    return list
  }, [servers, hasMultipleSources])

  useEffect(() => {
    if (debouncedSearchQuery.trim().length < 2) {
      setSearchResults([])
      return
    }
    const headers: HeadersInit = {}
    if (auth.token) headers['Authorization'] = `Bearer ${auth.token}`
    const ctrl = new AbortController()
    fetch(`/api/players?search=${encodeURIComponent(debouncedSearchQuery)}&limit=10`, {
      headers,
      signal: ctrl.signal,
    })
      .then((res) => (res.ok ? res.json() : []))
      .then((data: PlayerProfile[]) => setSearchResults(data ?? []))
      .catch(() => {
        /* aborted or network error */
      })
    return () => ctrl.abort()
  }, [debouncedSearchQuery, auth.token])

  // Select a player from search results
  const selectPlayer = (playerId: number) => {
    navigate(`/players/${playerId}`)
    setSearchResults([])
    setSearchQuery('')
  }

  // Hint text under the search input describes the current state so
  // the right pane never feels like it changed silently.
  const trimmed = searchQuery.trim()
  let searchHint: string
  if (trimmed.length === 0) {
    searchHint = id
      ? 'Type a name to find another player.'
      : 'Type at least two characters to search.'
  } else if (trimmed.length < 2) {
    searchHint = 'Keep typing…'
  } else if (searchResults.length === 0) {
    searchHint = 'No matches.'
  } else {
    searchHint = `${searchResults.length} match${searchResults.length === 1 ? '' : 'es'}`
  }

  // Right-pane content priority: live search results take precedence
  // (so users can jump between players without backing out), then the
  // selected player's stats, then a friendly empty state.
  const showResults = searchResults.length > 0
  const showStats = !showResults && id

  return (
    <div className="players-page">
      {id && stats?.player && (
        <Breadcrumbs
          crumbs={[
            { label: 'Players', to: '/players' },
            { label: <ColoredText text={stats.player.is_vr ? stripVRPrefix(stats.player.name) : stats.player.name} /> },
          ]}
        />
      )}

      <div className="players-layout">
        <aside className="players-search-rail">
          <input
            type="text"
            placeholder={auth.isAuthenticated ? "Search by name or GUID…" : "Search by name…"}
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="search-input players-search-rail__input"
            autoFocus
          />
          <p className="players-search-rail__hint">{searchHint}</p>
        </aside>

        <main className="players-results-area">
          {showResults ? (
            <div className="player-cards-grid">
              {searchResults.map((player) => {
                const displayName = player.is_vr ? stripVRPrefix(player.name) : player.name
                return (
                  <button
                    key={player.id}
                    type="button"
                    className="player-card"
                    onClick={() => selectPlayer(player.id)}
                  >
                    <span className="player-card__avatar">
                      <PlayerPortrait model={player.model} size="lg" />
                      {player.is_bot ? (
                        <BotBadge isBot skill={5} size="sm" />
                      ) : (
                        <PlayerBadge
                          isVerified={player.is_verified}
                          isAdmin={player.is_admin}
                          isVR={player.is_vr}
                          size="sm"
                        />
                      )}
                    </span>
                    <span className="player-card__name">
                      <ColoredText text={displayName} />
                    </span>
                    <span className="player-card__meta">
                      Last seen {formatDate(player.last_seen)}
                    </span>
                    {player.total_playtime_seconds > 0 && (
                      <span className="player-card__meta">
                        {formatDuration(player.total_playtime_seconds)} played
                      </span>
                    )}
                  </button>
                )
              })}
            </div>
          ) : showStats ? (
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

                  {auth.isAdmin && auth.token && !stats.player.is_bot && (
                    <PlayerSessions playerId={stats.player.id} token={auth.token} />
                  )}

                  <PlayerRecentMatches playerId={stats.player.id} />
                </div>
              ) : null}
            </div>
          ) : onlinePlayers.length > 0 ? (
            <section className="online-players">
              <h3 className="online-players__heading">
                <span className="online-players__dot" aria-hidden="true" />
                Playing now
                <span className="online-players__count">{onlinePlayers.length}</span>
              </h3>
              <div className="player-cards-grid">
                {onlinePlayers.map((p) => (
                  <button
                    key={p.key}
                    type="button"
                    className="player-card"
                    onClick={() => navigate(`/players/${p.playerId}`)}
                  >
                    <span className="player-card__avatar">
                      <PlayerPortrait model={p.model} size="lg" />
                      <PlayerBadge
                        isVerified={p.isVerified}
                        isAdmin={p.isAdmin}
                        isVR={p.isVR}
                        size="sm"
                      />
                    </span>
                    <span className="player-card__name">
                      <ColoredText text={p.isVR ? stripVRPrefix(p.name) : p.name} />
                    </span>
                    <span className="player-card__meta">{p.serverName}</span>
                  </button>
                ))}
              </div>
            </section>
          ) : (
            <div className="players-empty">
              <p>No humans playing right now. Search by name to view a player's statistics.</p>
            </div>
          )}
        </main>
      </div>
    </div>
  )
}
