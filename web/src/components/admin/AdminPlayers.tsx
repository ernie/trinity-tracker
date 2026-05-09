import { useState, useCallback, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../../hooks/useAuth'
import { useDebouncedValue } from '../../hooks/useDebouncedValue'
import { BotBadge } from '../BotBadge'
import { ColoredText } from '../ColoredText'
import { PlayerBadge } from '../PlayerBadge'
import { PlayerPortrait } from '../PlayerPortrait'
import { formatDate, formatDuration } from '../../utils/formatters'
import { stripVRPrefix } from '../../utils'
import type { PlayerProfile, PlayerGUID } from '../../types'

export function AdminPlayers() {
  const { auth } = useAuth()
  const token = auth.token!

  // Player search (target selection) — fires automatically as the admin types.
  const [searchQuery, setSearchQuery] = useState('')
  const [searchResults, setSearchResults] = useState<PlayerProfile[]>([])
  const debouncedSearchQuery = useDebouncedValue(searchQuery, 200)

  // Selected target player
  const [selected, setSelected] = useState<PlayerProfile | null>(null)
  const [guids, setGuids] = useState<PlayerGUID[]>([])
  const [loadingGuids, setLoadingGuids] = useState(false)

  // Merge state
  const [mergeQuery, setMergeQuery] = useState('')
  const [mergeResults, setMergeResults] = useState<PlayerProfile[]>([])
  const debouncedMergeQuery = useDebouncedValue(mergeQuery, 200)
  const [merging, setMerging] = useState(false)
  const [splitting, setSplitting] = useState<number | null>(null)
  const [error, setError] = useState('')

  const headers = { Authorization: `Bearer ${token}` }

  useEffect(() => {
    if (debouncedSearchQuery.trim().length < 2) return
    const ctrl = new AbortController()
    fetch(`/api/players?search=${encodeURIComponent(debouncedSearchQuery)}&limit=10`, {
      headers: { Authorization: `Bearer ${token}` },
      signal: ctrl.signal,
    })
      .then((res) => (res.ok ? res.json() : []))
      .then((data: PlayerProfile[]) => setSearchResults(data ?? []))
      .catch(() => {
        /* aborted or network error */
      })
    return () => ctrl.abort()
  }, [debouncedSearchQuery, token])

  // Hide stored results once the query is too short — store keeps the
  // last fetch's data so a fresh ≥2-char query overwrites cleanly.
  const displaySearchResults = debouncedSearchQuery.trim().length >= 2 ? searchResults : []

  const fetchGuids = useCallback(
    (playerId: number) => {
      setLoadingGuids(true)
      fetch(`/api/players/${playerId}/guids`, { headers })
        .then((res) => (res.ok ? res.json() : []))
        .then((data) => setGuids(data || []))
        .catch(() => setGuids([]))
        .finally(() => setLoadingGuids(false))
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [token],
  )

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (selected) fetchGuids(selected.id)
    else setGuids([])
  }, [selected, fetchGuids])

  const selectPlayer = (p: PlayerProfile) => {
    setSelected(p)
    setSearchResults([])
    setSearchQuery('')
    setMergeQuery('')
    setMergeResults([])
    setError('')
  }

  useEffect(() => {
    if (!selected || debouncedMergeQuery.trim().length < 2) return
    const ctrl = new AbortController()
    fetch(`/api/players?search=${encodeURIComponent(debouncedMergeQuery)}&limit=10`, {
      headers: { Authorization: `Bearer ${token}` },
      signal: ctrl.signal,
    })
      .then((res) => (res.ok ? res.json() : []))
      .then((data: PlayerProfile[]) => {
        const filtered = (data ?? []).filter((p) => p.id !== selected.id)
        setMergeResults(filtered)
      })
      .catch(() => {
        /* aborted or network error */
      })
    return () => ctrl.abort()
  }, [debouncedMergeQuery, selected, token])

  // Hide stored merge results when no player is selected or the merge
  // query is too short — fresh fetches overwrite when both are valid.
  const displayMergeResults = selected && debouncedMergeQuery.trim().length >= 2 ? mergeResults : []

  const handleMerge = async (mergePlayerId: number) => {
    if (!selected) return
    if (
      !confirm(
        'Are you sure you want to merge this player? This will move all their GUIDs and stats to the selected player.',
      )
    ) {
      return
    }

    setMerging(true)
    setError('')
    try {
      const res = await fetch(`/api/admin/players/${selected.id}/merge`, {
        method: 'POST',
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify({ merge_player_id: mergePlayerId }),
      })
      if (!res.ok) {
        const data = await res.json()
        throw new Error(data.error || 'Merge failed')
      }
      setMergeQuery('')
      setMergeResults([])
      fetchGuids(selected.id)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Merge failed')
    } finally {
      setMerging(false)
    }
  }

  const handleSplit = async (guidId: number) => {
    if (!confirm('Split this GUID into a separate player?')) return

    setSplitting(guidId)
    setError('')
    try {
      const res = await fetch(`/api/admin/guids/${guidId}/split`, {
        method: 'POST',
        headers,
      })
      if (!res.ok) {
        const data = await res.json()
        throw new Error(data.error || 'Split failed')
      }
      if (selected) fetchGuids(selected.id)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Split failed')
    } finally {
      setSplitting(null)
    }
  }

  // Right-pane priority mirrors PlayersPage: a fresh search hides the
  // selected detail panel so admins can pivot to a different player
  // without losing context. Gates on `displaySearchResults` so a stale
  // in-flight fetch resolving after a short-query gap can't reappear.
  const showResults = displaySearchResults.length > 0

  return (
    <div className="admin-players">
      <div className="admin-section-header">
        <h2>Player administration</h2>
      </div>

      <input
        type="text"
        className="admin-input"
        placeholder="Search players by name or GUID…"
        value={searchQuery}
        onChange={(e) => setSearchQuery(e.target.value)}
      />

      {showResults && (
        <div className="player-cards-grid" style={{ marginTop: 'var(--space-3)' }}>
          {displaySearchResults.map((p) => {
            const displayName = p.is_vr ? stripVRPrefix(p.name) : p.name
            return (
              <button
                key={p.id}
                type="button"
                className="player-card"
                onClick={() => selectPlayer(p)}
              >
                <PlayerPortrait model={p.model} size="lg" />
                <div className="player-card__body">
                  <span className="player-card__name">
                    {p.is_bot && <BotBadge isBot skill={5} />}
                    {!p.is_bot && (
                      <PlayerBadge
                        isVerified={p.is_verified}
                        isAdmin={p.is_admin}
                        isVR={p.is_vr}
                      />
                    )}
                    <ColoredText text={displayName} />
                  </span>
                  <span className="player-card__meta">
                    Last seen {formatDate(p.last_seen)}
                    {p.total_playtime_seconds > 0 &&
                      ` · ${formatDuration(p.total_playtime_seconds)} played`}
                  </span>
                </div>
              </button>
            )
          })}
        </div>
      )}

      {error && <div className="error-message" style={{ marginTop: 'var(--space-3)' }}>{error}</div>}

      {!showResults && selected && (
        <div className="admin-player-detail">
          <h3>
            <PlayerPortrait model={selected.model} size="lg" />
            <Link to={`/players/${selected.id}`}>
              <ColoredText text={selected.is_vr ? stripVRPrefix(selected.name) : selected.name} />
            </Link>
          </h3>

          <details className="filter-section" open>
            <summary className="filter-section__header">
              <span className="filter-section__caret" aria-hidden="true">▸</span>
              <span className="filter-section__title">
                Linked GUIDs
                <span className="admin-section-header__count"> ({guids.length})</span>
              </span>
            </summary>
            <div className="filter-section__body">
              {loadingGuids ? (
                <div className="admin-loading">Loading GUIDs…</div>
              ) : (
                <div className="guids-list">
                  {guids.map((guid) => (
                    <div key={guid.id} className="guid-item">
                      <div className="guid-info">
                        <ColoredText text={guid.name} />
                        <span className="guid-hash">{guid.guid}</span>
                        <span className="guid-dates">
                          {formatDate(guid.first_seen)} – {formatDate(guid.last_seen)}
                        </span>
                      </div>
                      {guids.length > 1 && (
                        <button
                          className="admin-btn-danger"
                          onClick={() => handleSplit(guid.id)}
                          disabled={splitting === guid.id}
                        >
                          {splitting === guid.id ? 'Splitting…' : 'Split'}
                        </button>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          </details>

          <details className="filter-section" open>
            <summary className="filter-section__header">
              <span className="filter-section__caret" aria-hidden="true">▸</span>
              <span className="filter-section__title">Merge another player into this one</span>
            </summary>
            <div className="filter-section__body">
              <div className="merge-search-input">
                <input
                  type="text"
                  placeholder="Search players by name or GUID…"
                  value={mergeQuery}
                  onChange={(e) => setMergeQuery(e.target.value)}
                />
              </div>
              {displayMergeResults.length > 0 && (
                <div className="merge-results">
                  {displayMergeResults.map((p) => (
                    <div key={p.id} className="merge-result-item">
                      <div className="merge-player-info">
                        <ColoredText text={p.name} />
                        <span className="merge-player-date">Last seen: {formatDate(p.last_seen)}</span>
                      </div>
                      <button className="admin-btn-danger" onClick={() => handleMerge(p.id)} disabled={merging}>
                        {merging ? 'Merging…' : 'Merge'}
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </details>
        </div>
      )}
    </div>
  )
}
