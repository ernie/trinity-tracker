import { useCallback, useEffect, useState } from 'react'
import { useAuth } from '../../hooks/useAuth'
import { heartbeatHealth, healthLabel } from '../../utils/sourceHealth'
import type { PendingRequest } from '../../types'
import { UserPicker, type UserOption } from './UserPicker'

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
  owner_user_id?: number
  owner_username?: string
  servers: ApprovedSourceServer[]
}

const SOURCE_NAME_PATTERN = /^[A-Za-z0-9_-]+$/

export function AdminSources() {
  const { auth } = useAuth()
  const token = auth.token!

  const [approved, setApproved] = useState<ApprovedSource[]>([])
  const [pending, setPending] = useState<PendingRequest[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [newSource, setNewSource] = useState('')
  // Default the owner to the admin themselves — self-service is the
  // common case. Pre-populated from /api/account/profile on mount.
  const [selectedOwner, setSelectedOwner] = useState<UserOption | null>(null)
  const [creating, setCreating] = useState(false)
  // transferTarget tracks which row's transfer-owner inline form is
  // open. null = closed. The picked user is kept here too so closing
  // the form discards an unsubmitted choice.
  const [transferTarget, setTransferTarget] = useState<string | null>(null)
  const [transferTo, setTransferTo] = useState<UserOption | null>(null)

  const newSourceTrimmed = newSource.trim()
  const newSourceValid = newSourceTrimmed !== '' && SOURCE_NAME_PATTERN.test(newSourceTrimmed)
  const formValid = newSourceValid && selectedOwner !== null

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

  const fetchPending = useCallback(async () => {
    try {
      const res = await fetch('/api/admin/sources/pending', {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) throw new Error(`pending: ${res.status}`)
      setPending((await res.json()) ?? [])
    } catch (e) {
      // Non-fatal: leave existing pending list, surface in error banner.
      setError(`Failed to load pending requests: ${(e as Error).message}`)
    }
  }, [token])

  // Pre-populate the create-form picker with the logged-in admin so
  // the common self-service case is one click away. Only on first
  // mount; subsequent picks are not overwritten.
  useEffect(() => {
    let cancelled = false
    fetch('/api/account/profile', { headers: { Authorization: `Bearer ${token}` } })
      .then((res) => (res.ok ? res.json() : null))
      .then((profile) => {
        if (cancelled || !profile?.user) return
        setSelectedOwner((prev) =>
          prev ?? {
            id: profile.user.id,
            username: profile.user.username,
            is_admin: profile.user.is_admin,
            player_name: profile.player?.name ?? null,
          },
        )
      })
      .catch(() => {
        /* non-fatal: picker just starts empty */
      })
    return () => {
      cancelled = true
    }
  }, [token])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    fetchAll()
    fetchPending()
    const id = setInterval(() => {
      fetchAll()
      fetchPending()
    }, 15_000)
    return () => clearInterval(id)
  }, [fetchAll, fetchPending])

  const approveRequest = async (req: PendingRequest) => {
    setError('')
    setNotice('')
    try {
      const res = await fetch(`/api/admin/sources/${encodeURIComponent(req.source)}/approve`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
      setNotice(`Approved ${req.source}.`)
      await Promise.all([fetchPending(), fetchAll()])
    } catch (err) {
      setError(`Approve failed: ${(err as Error).message}`)
    }
  }

  const rejectRequest = async (req: PendingRequest, reason: string) => {
    setError('')
    setNotice('')
    try {
      const res = await fetch(`/api/admin/sources/${encodeURIComponent(req.source)}/reject`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ reason }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || `${res.status} ${res.statusText}`)
      }
      setNotice(`Rejected ${req.source}.`)
      await fetchPending()
    } catch (err) {
      setError(`Reject failed: ${(err as Error).message}`)
    }
  }

  const renameRequest = async (req: PendingRequest, newName: string): Promise<string | null> => {
    setError('')
    setNotice('')
    try {
      const res = await fetch(`/api/admin/sources/${encodeURIComponent(req.source)}/rename`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ name: newName }),
      })
      if (!res.ok) {
        const text = await res.text()
        return text || `${res.status} ${res.statusText}`
      }
      setNotice(`Renamed ${req.source} → ${newName}.`)
      await fetchPending()
      return null
    } catch (err) {
      return (err as Error).message
    }
  }

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
    if (!selectedOwner) {
      setError('Pick an owner — every remote source must be assigned to a user.')
      return
    }
    setCreating(true)
    try {
      const res = await fetch('/api/admin/sources', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ source: newSourceTrimmed, owner_user_id: selectedOwner.id }),
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

  const transferOwner = async (source: ApprovedSource, newOwner: UserOption) => {
    setError('')
    setNotice('')
    try {
      const res = await fetch(`/api/admin/sources/${encodeURIComponent(source.source)}/owner`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ owner_user_id: newOwner.id }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || `${res.status} ${res.statusText}`)
      }
      setNotice(`Transferred ${source.source} to ${newOwner.username}.`)
      setTransferTarget(null)
      setTransferTo(null)
      await fetchAll()
    } catch (err) {
      setError(`Transfer failed: ${(err as Error).message}`)
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

      {pending.length > 0 && (
        <section className="admin-pending-requests">
          <h2>Pending requests ({pending.length})</h2>
          <p className="admin-help">
            Users requesting a self-service collector source. Approve mints
            credentials and flips the row to active; reject stores the reason
            for the requester to see in their next request modal.
          </p>
          <table className="admin-table">
            <thead>
              <tr>
                <th>User</th>
                <th>Source</th>
                <th>Purpose</th>
                <th>Submitted</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {pending.map((req) => (
                <PendingRow
                  key={req.source}
                  req={req}
                  onApprove={() => approveRequest(req)}
                  onReject={(reason) => rejectRequest(req, reason)}
                  onRename={(name) => renameRequest(req, name)}
                />
              ))}
            </tbody>
          </table>
        </section>
      )}

      <section className="admin-sources-create">
        <h2>Provision a new collector</h2>
        <p className="admin-help">
          Pick a short name — it becomes the NATS subject scope, and
          the remote operator configures it as{' '}
          <code>tracker.collector.source_id</code>. Every remote
          source has an owner; the default is you, but you can mint
          on another user's behalf and they'll see it under their
          My Servers.
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
          <div className="form-group">
            <label htmlFor="new-source-owner">Owner</label>
            <UserPicker
              token={token}
              selected={selectedOwner}
              onChange={setSelectedOwner}
              placeholder="Search users by name or linked player…"
              required
            />
            <p className="form-hint">
              The owner can rotate creds, leave the source, and RCON
              its servers without admin delegation.
            </p>
          </div>
          <div className="admin-sources-form-actions">
            <button type="submit" className="admin-btn" disabled={creating || !formValid}>
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
                <th>Owner</th>
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
                    <td data-label="Owner">
                      {s.is_remote ? (
                        transferTarget === s.source ? (
                          <form
                            className="admin-sources-transfer"
                            onSubmit={(e) => {
                              e.preventDefault()
                              if (transferTo && transferTo.id !== s.owner_user_id) {
                                transferOwner(s, transferTo)
                              }
                            }}
                          >
                            <UserPicker
                              token={token}
                              selected={transferTo}
                              onChange={setTransferTo}
                              placeholder="Search users…"
                              excludeUserId={s.owner_user_id}
                              autoFocus
                              required
                            />
                            <button
                              type="submit"
                              className="admin-btn admin-btn-small"
                              disabled={!transferTo || transferTo.id === s.owner_user_id}
                            >
                              Save
                            </button>
                            <button
                              type="button"
                              className="admin-btn admin-btn-small admin-btn-ghost"
                              onClick={() => {
                                setTransferTarget(null)
                                setTransferTo(null)
                              }}
                            >
                              Cancel
                            </button>
                          </form>
                        ) : (
                          <div className="admin-sources-owner">
                            <span>{s.owner_username || <span className="admin-muted">unassigned</span>}</span>
                            <button
                              type="button"
                              className="admin-btn admin-btn-small admin-btn-ghost"
                              onClick={() => {
                                setTransferTarget(s.source)
                                setTransferTo(null)
                              }}
                            >
                              Transfer
                            </button>
                          </div>
                        )
                      ) : (
                        <span className="admin-muted">—</span>
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

// PendingRow is a small inline component holding the per-row state
// for an admin's approve/reject/rename flow. Approve is a single
// click; reject opens an inline reason input; rename opens an
// inline name input — both have to be non-empty to submit. Returns
// an error string from rename; reject errors surface via parent's
// notice/error banners.
interface PendingRowProps {
  req: PendingRequest
  onApprove: () => void
  onReject: (reason: string) => void
  onRename: (newName: string) => Promise<string | null>
}

const NAME_PATTERN = /^[A-Za-z0-9_-]{3,16}$/

function PendingRow({ req, onApprove, onReject, onRename }: PendingRowProps) {
  const [mode, setMode] = useState<'idle' | 'rejecting' | 'renaming'>('idle')
  const [text, setText] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [renameError, setRenameError] = useState<string | null>(null)

  const reset = () => {
    setMode('idle')
    setText('')
    setRenameError(null)
  }

  const submitReject = async () => {
    if (!text.trim()) return
    setSubmitting(true)
    await onReject(text.trim())
    setSubmitting(false)
    reset()
  }

  const submitRename = async () => {
    setRenameError(null)
    if (!NAME_PATTERN.test(text)) {
      setRenameError('Name must be 3-16 characters: letters, numbers, _ or -.')
      return
    }
    setSubmitting(true)
    const err = await onRename(text)
    setSubmitting(false)
    if (err) {
      setRenameError(err)
      return
    }
    reset()
  }

  return (
    <tr>
      <td data-label="User">{req.owner_username}</td>
      <td data-label="Source">
        <div className="admin-sources-source-name">{req.source}</div>
      </td>
      <td data-label="Purpose">
        {req.requested_purpose || <span className="admin-muted">—</span>}
      </td>
      <td data-label="Submitted">{req.submitted_at}</td>
      <td data-label="Actions" className="admin-sources-actions">
        {mode === 'rejecting' && (
          <div className="admin-pending-inline-form">
            <input
              type="text"
              value={text}
              onChange={(e) => setText(e.target.value)}
              placeholder="Reason (shown to requester)"
              autoFocus
            />
            <button
              className="admin-btn admin-btn-danger"
              onClick={submitReject}
              disabled={submitting || !text.trim()}
            >
              {submitting ? '…' : 'Confirm reject'}
            </button>
            <button className="admin-btn" onClick={reset} disabled={submitting}>
              Cancel
            </button>
          </div>
        )}
        {mode === 'renaming' && (
          <div className="admin-pending-inline-form">
            <input
              type="text"
              value={text}
              onChange={(e) => setText(e.target.value)}
              placeholder="New name (e.g. mygame-jfk)"
              autoFocus
            />
            <button
              className="admin-btn"
              onClick={submitRename}
              disabled={submitting || !text.trim()}
            >
              {submitting ? '…' : 'Confirm rename'}
            </button>
            <button className="admin-btn" onClick={reset} disabled={submitting}>
              Cancel
            </button>
            {renameError && <div className="error-message">{renameError}</div>}
          </div>
        )}
        {mode === 'idle' && (
          <>
            <button className="admin-btn" onClick={onApprove}>
              Approve
            </button>
            <button
              className="admin-btn"
              onClick={() => {
                setMode('renaming')
                setText(req.source)
              }}
            >
              Rename
            </button>
            <button
              className="admin-btn admin-btn-danger"
              onClick={() => setMode('rejecting')}
            >
              Reject
            </button>
          </>
        )}
      </td>
    </tr>
  )
}
