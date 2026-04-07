import { useState, useCallback, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../../hooks/useAuth'
import { ColoredText } from '../ColoredText'
import { formatDate } from '../../utils/formatters'
import type { PlayerProfile, PlayerGUID } from '../../types'

export function AdminPlayers() {
  const { auth } = useAuth()
  const token = auth.token!

  // Player search (target selection)
  const [searchQuery, setSearchQuery] = useState('')
  const [searchResults, setSearchResults] = useState<PlayerProfile[]>([])
  const [searching, setSearching] = useState(false)

  // Selected target player
  const [selected, setSelected] = useState<PlayerProfile | null>(null)
  const [guids, setGuids] = useState<PlayerGUID[]>([])
  const [loadingGuids, setLoadingGuids] = useState(false)

  // Merge state
  const [mergeQuery, setMergeQuery] = useState('')
  const [mergeResults, setMergeResults] = useState<PlayerProfile[]>([])
  const [mergeSearching, setMergeSearching] = useState(false)
  const [merging, setMerging] = useState(false)
  const [splitting, setSplitting] = useState<number | null>(null)
  const [error, setError] = useState('')

  const headers = { Authorization: `Bearer ${token}` }

  const handleSearch = useCallback(() => {
    if (!searchQuery.trim()) {
      setSearchResults([])
      return
    }
    setSearching(true)
    fetch(`/api/players?search=${encodeURIComponent(searchQuery)}&limit=10`, { headers })
      .then((res) => res.json())
      .then((data) => setSearchResults(data || []))
      .catch(() => setSearchResults([]))
      .finally(() => setSearching(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchQuery, token])

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

  const handleMergeSearch = useCallback(() => {
    if (!mergeQuery.trim() || !selected) {
      setMergeResults([])
      return
    }
    setMergeSearching(true)
    fetch(`/api/players?search=${encodeURIComponent(mergeQuery)}&limit=10`, { headers })
      .then((res) => res.json())
      .then((data) => {
        const filtered = (data || []).filter((p: PlayerProfile) => p.id !== selected.id)
        setMergeResults(filtered)
      })
      .catch(() => setMergeResults([]))
      .finally(() => setMergeSearching(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mergeQuery, selected, token])

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

  return (
    <div className="admin-players">
      <div className="admin-section-header">
        <h2>Player Administration</h2>
      </div>

      <div className="admin-players-search">
        <input
          type="text"
          placeholder="Search players by name or GUID…"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
        />
        <button onClick={handleSearch} disabled={searching}>
          {searching ? 'Searching…' : 'Search'}
        </button>
      </div>

      {searchResults.length > 0 && (
        <div className="search-results">
          {searchResults.map((p) => (
            <div key={p.id} className="search-result-item" onClick={() => selectPlayer(p)}>
              <ColoredText text={p.name} />
              <span className="player-last-seen">Last seen: {formatDate(p.last_seen)}</span>
            </div>
          ))}
        </div>
      )}

      {error && <div className="error-message">{error}</div>}

      {selected && (
        <div className="admin-player-detail">
          <h3>
            <Link to={`/players/${selected.id}`}>
              <ColoredText text={selected.name} />
            </Link>
          </h3>

          <div className="admin-subsection">
            <h4>Linked GUIDs ({guids.length})</h4>
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
                        className="split-btn"
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

          <div className="admin-subsection">
            <h4>Merge Another Player Into This One</h4>
            <div className="merge-search-input">
              <input
                type="text"
                placeholder="Search players by name or GUID…"
                value={mergeQuery}
                onChange={(e) => setMergeQuery(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleMergeSearch()}
              />
              <button onClick={handleMergeSearch} disabled={mergeSearching}>
                {mergeSearching ? 'Searching…' : 'Search'}
              </button>
            </div>
            {mergeResults.length > 0 && (
              <div className="merge-results">
                {mergeResults.map((p) => (
                  <div key={p.id} className="merge-result-item">
                    <div className="merge-player-info">
                      <ColoredText text={p.name} />
                      <span className="merge-player-date">Last seen: {formatDate(p.last_seen)}</span>
                    </div>
                    <button className="merge-btn" onClick={() => handleMerge(p.id)} disabled={merging}>
                      {merging ? 'Merging…' : 'Merge'}
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
