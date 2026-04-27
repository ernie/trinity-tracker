import { useCallback, useEffect, useState } from 'react'
import { useAuth } from '../../hooks/useAuth'

type ApprovedSourceServer = {
  id: number
  local_id: number
  key: string
  address: string
  active: boolean
}

type ApprovedSource = {
  source: string
  version?: string
  demo_base_url?: string
  last_heartbeat_at?: string
  is_remote: boolean
  active: boolean
  servers: ApprovedSourceServer[]
}

const SOURCE_NAME_PATTERN = /^[A-Za-z0-9_-]+$/

// Thresholds match the design spec: <90s healthy, 90s–10min stale,
// >10min gone.
const STALE_THRESHOLD_MS = 90 * 1000
const GONE_THRESHOLD_MS = 10 * 60 * 1000

type Health = 'green' | 'stale' | 'gone' | 'unknown'

function heartbeatHealth(lastHeartbeatAt?: string): Health {
  if (!lastHeartbeatAt) return 'unknown'
  const ts = Date.parse(lastHeartbeatAt)
  if (!Number.isFinite(ts)) return 'unknown'
  const age = Date.now() - ts
  if (age < STALE_THRESHOLD_MS) return 'green'
  if (age < GONE_THRESHOLD_MS) return 'stale'
  return 'gone'
}

function healthLabel(h: Health): string {
  switch (h) {
    case 'green': return 'healthy'
    case 'stale': return 'stale'
    case 'gone': return 'gone'
    default: return 'unknown'
  }
}

export function AdminSources() {
  const { auth } = useAuth()
  const token = auth.token!

  const [approved, setApproved] = useState<ApprovedSource[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [newSource, setNewSource] = useState('')
  const [creating, setCreating] = useState(false)

  const newSourceTrimmed = newSource.trim()
  const newSourceValid = newSourceTrimmed !== '' && SOURCE_NAME_PATTERN.test(newSourceTrimmed)

  const fetchAll = useCallback(async () => {
    try {
      const res = await fetch('/api/admin/sources', { headers: { Authorization: `Bearer ${token}` } })
      if (!res.ok) throw new Error(`sources: ${res.status}`)
      setApproved((await res.json()) ?? [])
      setError('')
    } catch (e) {
      setError(`Failed to load sources: ${(e as Error).message}`)
    } finally {
      setLoading(false)
    }
  }, [token])

  useEffect(() => {
    fetchAll()
    const id = setInterval(fetchAll, 15_000)
    return () => clearInterval(id)
  }, [fetchAll])

  const downloadCredsBlob = async (res: Response, source: string) => {
    const blob = await res.blob()
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `${source}.creds`
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    URL.revokeObjectURL(url)
  }

  const createSource = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setNotice('')
    if (!newSourceValid) {
      setError('Source name must contain only letters, digits, hyphen, or underscore.')
      return
    }
    setCreating(true)
    try {
      const res = await fetch('/api/admin/sources', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ source: newSourceTrimmed }),
      })
      if (!res.ok) {
        throw new Error(`${res.status} ${res.statusText}`)
      }
      setNotice(`Created ${newSourceTrimmed}.`)
      setNewSource('')
      await fetchAll()
    } catch (err) {
      setError(`Create failed: ${(err as Error).message}`)
    } finally {
      setCreating(false)
    }
  }

  const deactivateSource = async (source: ApprovedSource) => {
    if (!confirm(`Deactivate source ${source.source}? Creds are revoked immediately and the collector will be dropped from NATS. Historical matches keep their server reference but will display as inactive.`)) {
      return
    }
    setError('')
    setNotice('')
    try {
      const res = await fetch(`/api/admin/sources/${encodeURIComponent(source.source)}/deactivate`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) {
        throw new Error(`${res.status} ${res.statusText}`)
      }
      setNotice(`Deactivated ${source.source}.`)
      await fetchAll()
    } catch (err) {
      setError(`Deactivate failed: ${(err as Error).message}`)
    }
  }

  const reactivateSource = async (source: ApprovedSource) => {
    if (!confirm(`Reactivate source ${source.source}? Its servers come back online; you'll need to redownload creds and ship them to the collector.`)) {
      return
    }
    setError('')
    setNotice('')
    try {
      const res = await fetch(`/api/admin/sources/${encodeURIComponent(source.source)}/reactivate`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) {
        throw new Error(`${res.status} ${res.statusText}`)
      }
      setNotice(`Reactivated ${source.source}. Rotate creds to get a fresh .creds file.`)
      await fetchAll()
    } catch (err) {
      setError(`Reactivate failed: ${(err as Error).message}`)
    }
  }

  const rotateCreds = async (source: ApprovedSource) => {
    if (!confirm(`Rotate credentials for ${source.source}? The collector will need the new .creds file to reconnect.`)) {
      return
    }
    setError('')
    setNotice('')
    try {
      const res = await fetch(`/api/admin/sources/${encodeURIComponent(source.source)}/rotate-creds`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) {
        throw new Error(`${res.status} ${res.statusText}`)
      }
      await downloadCredsBlob(res, source.source)
      setNotice(`Rotated credentials for ${source.source}. New creds downloaded.`)
    } catch (err) {
      setError(`Rotate failed: ${(err as Error).message}`)
    }
  }

  const downloadExisting = async (source: ApprovedSource) => {
    setError('')
    try {
      const res = await fetch(`/api/admin/sources/${encodeURIComponent(source.source)}/creds`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) {
        throw new Error(`${res.status} ${res.statusText}`)
      }
      await downloadCredsBlob(res, source.source)
    } catch (err) {
      setError(`Download failed: ${(err as Error).message}`)
    }
  }

  if (loading) {
    return <div>Loading sources…</div>
  }

  return (
    <div className="admin-sources">
      {error && <div className="error-message">{error}</div>}
      {notice && <div className="notice-message">{notice}</div>}

      <section className="admin-sources-create">
        <h2>Provision a new collector</h2>
        <p className="admin-help">
          Pick a short name — it becomes the NATS subject scope, and
          the remote operator configures it as{' '}
          <code>tracker.collector.source_id</code>.
        </p>
        <form className="admin-sources-form" onSubmit={createSource}>
          <div className="form-group">
            <label htmlFor="new-source">Source name</label>
            <input
              id="new-source"
              type="text"
              value={newSource}
              onChange={(e) => setNewSource(e.target.value)}
              placeholder="remote-1"
              autoComplete="off"
              spellCheck={false}
              required
            />
            <p className="form-hint">
              Letters, digits, <code>-</code>, and <code>_</code> only.
            </p>
          </div>
          <div className="admin-sources-form-actions">
            <button type="submit" className="admin-btn" disabled={creating || !newSourceValid}>
              {creating ? 'Creating…' : 'Create source'}
            </button>
          </div>
        </form>
      </section>

      <section>
        <h2>Provisioned sources ({approved.length})</h2>
        {approved.length === 0 ? (
          <p className="admin-empty">No sources yet.</p>
        ) : (
          <table className="admin-table">
            <thead>
              <tr>
                <th>Health</th>
                <th>Source</th>
                <th>Version</th>
                <th>Last heartbeat</th>
                <th>Servers</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {approved.map((s) => {
                const h = heartbeatHealth(s.last_heartbeat_at)
                return (
                  <tr key={s.source}>
                    <td data-label="Health">
                      <span className={`health-badge health-${h}`}>{healthLabel(h)}</span>
                    </td>
                    <td data-label="Source">
                      <div className="admin-sources-source-name">
                        {s.source || (s.is_remote ? '—' : 'local')}
                      </div>
                      {s.demo_base_url ? (
                        <div className="admin-sources-demo-url" title="demo_base_url reported by the collector">
                          <code>{s.demo_base_url}</code>
                        </div>
                      ) : (
                        <div className="admin-sources-demo-url admin-muted">
                          no demo URL reported yet
                        </div>
                      )}
                    </td>
                    <td data-label="Version">{s.version || '—'}</td>
                    <td data-label="Last heartbeat">{s.last_heartbeat_at ?? '—'}</td>
                    <td data-label="Servers">
                      {s.servers.length === 0 ? (
                        <span className="admin-muted">awaiting first registration</span>
                      ) : (
                        <ul className="admin-server-list">
                          {s.servers.map((srv) => (
                            <li key={srv.id}>
                              {srv.key} <code>{srv.address}</code>
                            </li>
                          ))}
                        </ul>
                      )}
                    </td>
                    <td data-label="Actions" className="admin-sources-actions">
                      {s.is_remote ? (
                        s.active ? (
                          <>
                            <button className="admin-btn" onClick={() => downloadExisting(s)}>Download creds</button>
                            <button className="admin-btn" onClick={() => rotateCreds(s)}>Rotate creds</button>
                            <button className="admin-btn admin-btn-danger" onClick={() => deactivateSource(s)}>Deactivate</button>
                          </>
                        ) : (
                          <button className="admin-btn" onClick={() => reactivateSource(s)}>Reactivate</button>
                        )
                      ) : (
                        <span className="admin-muted">hub-local</span>
                      )}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
      </section>
    </div>
  )
}
