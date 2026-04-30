import { useState, useEffect, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../../hooks/useAuth'
import { ColoredText } from '../ColoredText'
import { formatDate, formatDuration } from '../../utils/formatters'
import type { AdminSession, Server, PlayerProfile } from '../../types'

const PAGE_SIZE = 50

export function AdminSessions() {
  const { auth } = useAuth()
  const token = auth.token!

  const [servers, setServers] = useState<Server[]>([])
  const [serverFilter, setServerFilter] = useState<number | null>(null)
  const [playerFilter, setPlayerFilter] = useState<{ id: number; label: string } | null>(null)

  const [playerSearch, setPlayerSearch] = useState('')
  const [playerResults, setPlayerResults] = useState<PlayerProfile[]>([])

  const [sessions, setSessions] = useState<AdminSession[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [hasMore, setHasMore] = useState(false)

  // Load server list for the dropdown
  useEffect(() => {
    fetch('/api/servers', { headers: { Authorization: `Bearer ${token}` } })
      .then((res) => (res.ok ? res.json() : []))
      .then((data) => setServers(data || []))
      .catch(() => setServers([]))
  }, [token])

  const buildUrl = useCallback(
    (beforeID?: number) => {
      const params = new URLSearchParams()
      params.set('limit', String(PAGE_SIZE))
      if (serverFilter) params.set('server_id', String(serverFilter))
      if (playerFilter) params.set('player_id', String(playerFilter.id))
      if (beforeID) params.set('before', String(beforeID))
      return `/api/admin/sessions?${params.toString()}`
    },
    [serverFilter, playerFilter],
  )

  const loadFirstPage = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const res = await fetch(buildUrl(), {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) {
        const data = await res.json().catch(() => ({}))
        throw new Error(data.error || 'Failed to load sessions')
      }
      const data: AdminSession[] = (await res.json()) || []
      setSessions(data)
      setHasMore(data.length === PAGE_SIZE)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load sessions')
      setSessions([])
      setHasMore(false)
    } finally {
      setLoading(false)
    }
  }, [buildUrl, token])

  const loadMore = useCallback(async () => {
    if (loading || sessions.length === 0) return
    const lastID = sessions[sessions.length - 1].id
    setLoading(true)
    try {
      const res = await fetch(buildUrl(lastID), {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) throw new Error('Failed to load more')
      const data: AdminSession[] = (await res.json()) || []
      setSessions((prev) => [...prev, ...data])
      setHasMore(data.length === PAGE_SIZE)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load more')
    } finally {
      setLoading(false)
    }
  }, [buildUrl, token, sessions, loading])

  // Refresh whenever filters change
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    loadFirstPage()
  }, [loadFirstPage])

  const handlePlayerSearch = async (q: string) => {
    setPlayerSearch(q)
    if (q.length < 2) {
      setPlayerResults([])
      return
    }
    try {
      const res = await fetch(`/api/players?search=${encodeURIComponent(q)}&limit=10`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (res.ok) setPlayerResults((await res.json()) || [])
    } catch {
      // ignore
    }
  }

  const selectPlayerFilter = (p: PlayerProfile) => {
    setPlayerFilter({ id: p.id, label: p.clean_name })
    setPlayerSearch('')
    setPlayerResults([])
  }

  return (
    <div className="admin-sessions">
      <div className="admin-section-header">
        <h2>Player Sessions</h2>
      </div>

      <div className="admin-filters">
        <div className="admin-filter">
          <label>Server</label>
          <select
            value={serverFilter ?? ''}
            onChange={(e) => setServerFilter(e.target.value ? Number(e.target.value) : null)}
          >
            <option value="">All servers</option>
            {servers.map((s) => (
              <option key={s.id} value={s.id}>
                {s.source} / {s.key}
              </option>
            ))}
          </select>
        </div>

        <div className="admin-filter">
          <label>Player</label>
          {playerFilter ? (
            <div className="selected-player">
              {playerFilter.label}
              <button type="button" onClick={() => setPlayerFilter(null)}>
                Clear
              </button>
            </div>
          ) : (
            <div className="player-typeahead">
              <input
                type="text"
                placeholder="Search players…"
                value={playerSearch}
                onChange={(e) => handlePlayerSearch(e.target.value)}
              />
              {playerResults.length > 0 && (
                <ul className="player-results">
                  {playerResults.map((p) => (
                    <li key={p.id} onClick={() => selectPlayerFilter(p)}>
                      {p.clean_name}
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </div>
      </div>

      {error && <div className="error-message">{error}</div>}

      <table className="sessions-table">
        <thead>
          <tr>
            <th>Player</th>
            <th>Server</th>
            <th>Joined</th>
            <th>Left</th>
            <th>Duration</th>
            <th>IP</th>
            <th>Client</th>
          </tr>
        </thead>
        <tbody>
          {sessions.map((s) => (
            <tr key={s.id}>
              <td>
                <Link to={`/players/${s.player_id}`}>
                  <ColoredText text={s.player_name} />
                </Link>
              </td>
              <td>{s.server_source} / {s.server_key}</td>
              <td>{formatDate(s.joined_at)}</td>
              <td>{s.left_at ? formatDate(s.left_at) : <em>active</em>}</td>
              <td>{s.duration_seconds ? formatDuration(s.duration_seconds) : '—'}</td>
              <td>{s.ip_address || '—'}</td>
              <td>
                {s.client_engine ? `${s.client_engine}${s.client_version ? ` ${s.client_version}` : ''}` : '—'}
              </td>
            </tr>
          ))}
          {sessions.length === 0 && !loading && (
            <tr>
              <td colSpan={7} className="admin-empty">
                No sessions found.
              </td>
            </tr>
          )}
        </tbody>
      </table>

      <div className="admin-pagination">
        {loading && <span>Loading…</span>}
        {!loading && hasMore && <button onClick={loadMore}>Load more</button>}
      </div>
    </div>
  )
}
