import { useState, useEffect, useCallback, useMemo } from 'react'
import { useAuth } from '../../hooks/useAuth'
import { formatDateTime } from '../../utils/formatters'
import { timeAgo } from '../../utils/sourceHealth'

interface AuditEntry {
  id: number
  source: string
  actor_user_id?: number
  actor_username?: string
  action: string
  detail?: string
  created_at: string
}

const DEFAULT_LIMIT = 100

export function AdminAudit() {
  const { auth } = useAuth()
  const token = auth.token!

  const [rows, setRows] = useState<AuditEntry[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const [sourceFilter, setSourceFilter] = useState('')
  const [actorFilter, setActorFilter] = useState('')
  const [actionFilter, setActionFilter] = useState('')
  const [sinceFilter, setSinceFilter] = useState('')

  const url = useMemo(() => {
    const params = new URLSearchParams()
    params.set('limit', String(DEFAULT_LIMIT))
    if (sourceFilter) params.set('source', sourceFilter)
    if (actorFilter) params.set('actor', actorFilter)
    if (actionFilter) params.set('action', actionFilter)
    if (sinceFilter) {
      // <input type="datetime-local"> gives "YYYY-MM-DDTHH:mm" without
      // a timezone; the backend wants RFC3339, so we anchor it to the
      // browser's local zone and convert to UTC.
      const d = new Date(sinceFilter)
      if (!isNaN(d.getTime())) params.set('since', d.toISOString())
    }
    return `/api/admin/audit?${params.toString()}`
  }, [sourceFilter, actorFilter, actionFilter, sinceFilter])

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const res = await fetch(url, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) {
        const data = await res.json().catch(() => ({}))
        throw new Error(data.error || `Failed to load audit (${res.status})`)
      }
      const data: AuditEntry[] = (await res.json()) || []
      setRows(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load audit')
      setRows([])
    } finally {
      setLoading(false)
    }
  }, [url, token])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load()
  }, [load])

  const distinctSources = useMemo(
    () => Array.from(new Set(rows.map((r) => r.source))).sort(),
    [rows],
  )
  const distinctActors = useMemo(
    () => Array.from(new Set(rows.map((r) => r.actor_username).filter(Boolean) as string[])).sort(),
    [rows],
  )
  const distinctActions = useMemo(
    () => Array.from(new Set(rows.map((r) => r.action))).sort(),
    [rows],
  )

  const clearFilters = () => {
    setSourceFilter('')
    setActorFilter('')
    setActionFilter('')
    setSinceFilter('')
  }

  return (
    <div className="admin-audit">
      <div className="admin-section-header">
        <h2>Audit Log</h2>
      </div>

      <div className="admin-audit-filters">
        <label>
          Source
          <select value={sourceFilter} onChange={(e) => setSourceFilter(e.target.value)}>
            <option value="">All</option>
            {distinctSources.map((s) => (
              <option key={s} value={s}>{s}</option>
            ))}
          </select>
        </label>
        <label>
          Actor
          <select value={actorFilter} onChange={(e) => setActorFilter(e.target.value)}>
            <option value="">All</option>
            {distinctActors.map((a) => (
              <option key={a} value={a}>{a}</option>
            ))}
          </select>
        </label>
        <label>
          Action
          <select value={actionFilter} onChange={(e) => setActionFilter(e.target.value)}>
            <option value="">All</option>
            {distinctActions.map((a) => (
              <option key={a} value={a}>{a}</option>
            ))}
          </select>
        </label>
        <label>
          Since
          <input
            type="datetime-local"
            value={sinceFilter}
            onChange={(e) => setSinceFilter(e.target.value)}
          />
        </label>
        <button type="button" onClick={clearFilters} className="admin-audit-clear">
          Clear
        </button>
        <button type="button" onClick={load} disabled={loading} className="admin-audit-refresh">
          {loading ? 'Loading…' : 'Refresh'}
        </button>
      </div>

      {error && <div className="admin-audit-error">{error}</div>}

      <table className="admin-audit-table">
        <thead>
          <tr>
            <th>When</th>
            <th>Source</th>
            <th>Actor</th>
            <th>Action</th>
            <th>Detail</th>
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 && !loading && (
            <tr>
              <td colSpan={5} className="admin-audit-empty">No audit rows match.</td>
            </tr>
          )}
          {rows.map((r) => (
            <tr key={r.id}>
              <td title={formatDateTime(r.created_at)}>{timeAgo(r.created_at)}</td>
              <td>{r.source}</td>
              <td>{r.actor_username || <span className="admin-audit-system">system</span>}</td>
              <td><span className={`admin-audit-action admin-audit-action-${r.action}`}>{r.action}</span></td>
              <td>{r.detail || ''}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
