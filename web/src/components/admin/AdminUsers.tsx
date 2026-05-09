import { useState, useEffect, FormEvent } from 'react'
import { useAuth } from '../../hooks/useAuth'
import { useDebouncedValue } from '../../hooks/useDebouncedValue'
import { ColoredText } from '../ColoredText'
import { PlayerStatsModal } from '../PlayerStatsModal'
import type { User, PlayerProfile } from '../../types'

export function AdminUsers() {
  const { auth } = useAuth()
  const token = auth.token!
  const currentUsername = auth.username!

  const [users, setUsers] = useState<User[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showCreateForm, setShowCreateForm] = useState(false)
  const [createUsername, setCreateUsername] = useState('')
  const [createPassword, setCreatePassword] = useState('')
  const [createIsAdmin, setCreateIsAdmin] = useState(false)
  const [createPlayerId, setCreatePlayerId] = useState<number | null>(null)
  const [resetPasswordUserId, setResetPasswordUserId] = useState<number | null>(null)
  const [newPassword, setNewPassword] = useState('')
  const [playerSearch, setPlayerSearch] = useState('')
  const [playerResults, setPlayerResults] = useState<PlayerProfile[]>([])
  const [editingUserId, setEditingUserId] = useState<number | null>(null)
  const [editPlayerSearch, setEditPlayerSearch] = useState('')
  const [editPlayerResults, setEditPlayerResults] = useState<PlayerProfile[]>([])
  const [editPlayerId, setEditPlayerId] = useState<number | null>(null)
  const [modalPlayer, setModalPlayer] = useState<{ id: number; name: string } | null>(null)

  const fetchUsers = async () => {
    try {
      const res = await fetch('/api/users', {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (res.ok) {
        setUsers(await res.json())
      } else {
        setError('Failed to load users')
      }
    } catch {
      setError('Failed to load users')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    fetchUsers()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token])

  const handleCreateUser = async (e: FormEvent) => {
    e.preventDefault()
    setError('')

    if (createPassword.length < 8) {
      setError('Password must be at least 8 characters')
      return
    }

    try {
      const res = await fetch('/api/users', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          username: createUsername,
          password: createPassword,
          is_admin: createIsAdmin,
          player_id: createPlayerId,
        }),
      })

      if (res.ok) {
        setShowCreateForm(false)
        setCreateUsername('')
        setCreatePassword('')
        setCreateIsAdmin(false)
        setCreatePlayerId(null)
        setPlayerSearch('')
        fetchUsers()
      } else {
        const data = await res.json()
        setError(data.error || 'Failed to create user')
      }
    } catch {
      setError('Network error')
    }
  }

  const handleDeleteUser = async (username: string) => {
    if (!confirm(`Delete user "${username}"?`)) return

    try {
      const res = await fetch(`/api/users/${username}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${token}` },
      })

      if (res.ok) {
        fetchUsers()
      } else {
        const data = await res.json()
        setError(data.error || 'Failed to delete user')
      }
    } catch {
      setError('Network error')
    }
  }

  const handleResetPassword = async (userId: number) => {
    if (newPassword.length < 8) {
      setError('Password must be at least 8 characters')
      return
    }

    try {
      const res = await fetch(`/api/users/${userId}/reset-password`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ new_password: newPassword }),
      })

      if (res.ok) {
        setResetPasswordUserId(null)
        setNewPassword('')
        fetchUsers()
      } else {
        const data = await res.json()
        setError(data.error || 'Failed to reset password')
      }
    } catch {
      setError('Network error')
    }
  }

  // Both player-search inputs debounce against this hook so a fast typist
  // fires one fetch per pause, not one per keystroke.
  const debouncedPlayerSearch = useDebouncedValue(playerSearch, 200)
  const debouncedEditPlayerSearch = useDebouncedValue(editPlayerSearch, 200)

  useEffect(() => {
    if (debouncedPlayerSearch.length < 2) return
    const ctrl = new AbortController()
    fetch(`/api/players?search=${encodeURIComponent(debouncedPlayerSearch)}&limit=10`, {
      headers: { Authorization: `Bearer ${token}` },
      signal: ctrl.signal,
    })
      .then((res) => (res.ok ? res.json() : []))
      .then((players: PlayerProfile[]) => setPlayerResults(players ?? []))
      .catch(() => {
        /* aborted or network error */
      })
    return () => ctrl.abort()
  }, [debouncedPlayerSearch, token])

  useEffect(() => {
    if (debouncedEditPlayerSearch.length < 2) return
    const ctrl = new AbortController()
    fetch(`/api/players?search=${encodeURIComponent(debouncedEditPlayerSearch)}&limit=10`, {
      headers: { Authorization: `Bearer ${token}` },
      signal: ctrl.signal,
    })
      .then((res) => (res.ok ? res.json() : []))
      .then((players: PlayerProfile[]) => setEditPlayerResults(players ?? []))
      .catch(() => {
        /* aborted or network error */
      })
    return () => ctrl.abort()
  }, [debouncedEditPlayerSearch, token])

  // Hide stored results once the corresponding query is too short.
  const displayPlayerResults = debouncedPlayerSearch.length >= 2 ? playerResults : []
  const displayEditPlayerResults = debouncedEditPlayerSearch.length >= 2 ? editPlayerResults : []

  const startEditingUser = (user: User) => {
    setEditingUserId(user.id)
    setEditPlayerId(user.player_id)
    setEditPlayerSearch('')
    setEditPlayerResults([])
  }

  const cancelEditing = () => {
    setEditingUserId(null)
    setEditPlayerId(null)
    setEditPlayerSearch('')
    setEditPlayerResults([])
  }

  const handleUpdatePlayerLink = async (userId: number) => {
    setError('')
    try {
      const res = await fetch(`/api/users/${userId}`, {
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ player_id: editPlayerId }),
      })

      if (res.ok) {
        cancelEditing()
        fetchUsers()
      } else {
        const data = await res.json()
        setError(data.error || 'Failed to update user')
      }
    } catch {
      setError('Network error')
    }
  }

  if (loading) {
    return <div className="admin-loading">Loading users…</div>
  }

  return (
    <div className="admin-users">
      <div className="admin-section-header">
        <h2>Users<span className="admin-section-header__count"> ({users.length})</span></h2>
      </div>

      {error && <div className="error-message">{error}</div>}

      {/* Create-user form lives behind a <details> toggle so the table stays
          the page's primary surface; the warm-orange caret on the summary
          rotates 90° when open, mirroring the structured-filter recipe. */}
      <details
        open={showCreateForm}
        onToggle={(e) => setShowCreateForm((e.target as HTMLDetailsElement).open)}
      >
        <summary className="admin-form-toggle">
          <span className="admin-form-toggle__caret" aria-hidden="true">▸</span>
          {showCreateForm ? 'Cancel' : 'Create user'}
        </summary>
        <form className="admin-form-card" onSubmit={handleCreateUser}>
          <div className="form-group">
            <label>Username</label>
            <input
              type="text"
              value={createUsername}
              onChange={(e) => setCreateUsername(e.target.value)}
              required
            />
          </div>
          <div className="form-group">
            <label>Temporary Password</label>
            <input
              type="password"
              value={createPassword}
              onChange={(e) => setCreatePassword(e.target.value)}
              required
              minLength={8}
            />
          </div>
          <div className="form-group checkbox">
            <label>
              <input
                type="checkbox"
                checked={createIsAdmin}
                onChange={(e) => setCreateIsAdmin(e.target.checked)}
              />
              Admin privileges
            </label>
          </div>
          <div className="form-group">
            <label>Link to Player (optional)</label>
            <input
              type="text"
              placeholder="Search players…"
              value={playerSearch}
              onChange={(e) => setPlayerSearch(e.target.value)}
            />
            {displayPlayerResults.length > 0 && (
              <ul className="player-results">
                {displayPlayerResults.map((p) => (
                  <li
                    key={p.id}
                    onClick={() => {
                      setCreatePlayerId(p.id)
                      setPlayerSearch(p.clean_name)
                      setPlayerResults([])
                    }}
                  >
                    {p.clean_name}
                  </li>
                ))}
              </ul>
            )}
            {createPlayerId && (
              <div className="selected-player">
                <span>Selected: Player #{createPlayerId}</span>
                <button
                  type="button"
                  onClick={() => {
                    setCreatePlayerId(null)
                    setPlayerSearch('')
                  }}
                >
                  Clear
                </button>
              </div>
            )}
          </div>
          <button type="submit" className="admin-btn-primary">Create user</button>
        </form>
      </details>

      <table className="admin-data-table users-table">
        <thead>
          <tr>
            <th>Username</th>
            <th>Role</th>
            <th>Player</th>
            <th>Pwd Change</th>
            <th>Last Login</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {users.map((user) => (
            <tr key={user.id}>
              <td data-label="Username">{user.username}</td>
              <td data-label="Role">{user.is_admin ? 'Admin' : 'User'}</td>
              <td data-label="Player">
                {editingUserId === user.id ? (
                  <div className="edit-player-inline">
                    <input
                      type="text"
                      placeholder="Search players…"
                      value={editPlayerSearch}
                      onChange={(e) => setEditPlayerSearch(e.target.value)}
                    />
                    {displayEditPlayerResults.length > 0 && (
                      <ul className="player-results">
                        {displayEditPlayerResults.map((p) => (
                          <li
                            key={p.id}
                            onClick={() => {
                              setEditPlayerId(p.id)
                              setEditPlayerSearch(p.clean_name)
                              setEditPlayerResults([])
                            }}
                          >
                            {p.clean_name}
                          </li>
                        ))}
                      </ul>
                    )}
                    {editPlayerId && (
                      <div className="selected-player">
                        <span>Player #{editPlayerId}</span>
                        <button
                          type="button"
                          onClick={() => {
                            setEditPlayerId(null)
                            setEditPlayerSearch('')
                          }}
                        >
                          Clear
                        </button>
                      </div>
                    )}
                    <div className="edit-actions">
                      <button className="admin-btn-primary" onClick={() => handleUpdatePlayerLink(user.id)}>Save</button>
                      <button className="admin-btn" onClick={cancelEditing}>Cancel</button>
                    </div>
                  </div>
                ) : (
                  <span className="player-link-display">
                    {user.player_id && user.player_name ? (
                      <button
                        type="button"
                        className="player-name-link"
                        onClick={() =>
                          setModalPlayer({ id: user.player_id!, name: user.player_name! })
                        }
                      >
                        <ColoredText text={user.player_name} />
                      </button>
                    ) : (
                      <span className="admin-muted">—</span>
                    )}
                    <button className="edit-link-btn" onClick={() => startEditingUser(user)}>
                      Edit
                    </button>
                  </span>
                )}
              </td>
              <td data-label="Pwd Change">{user.password_change_required ? 'Yes' : 'No'}</td>
              <td data-label="Last Login">{user.last_login ? new Date(user.last_login).toLocaleDateString() : 'Never'}</td>
              <td data-label="Actions" className="actions">
                {resetPasswordUserId === user.id ? (
                  <div className="reset-password-inline">
                    <input
                      type="password"
                      placeholder="New password"
                      value={newPassword}
                      onChange={(e) => setNewPassword(e.target.value)}
                    />
                    <button className="admin-btn-primary" onClick={() => handleResetPassword(user.id)}>Save</button>
                    <button
                      className="admin-btn"
                      onClick={() => {
                        setResetPasswordUserId(null)
                        setNewPassword('')
                      }}
                    >
                      Cancel
                    </button>
                  </div>
                ) : (
                  <>
                    <button className="admin-btn" onClick={() => setResetPasswordUserId(user.id)}>Reset Pwd</button>
                    {user.username !== currentUsername && (
                      <button
                        className="admin-btn-danger"
                        onClick={() => handleDeleteUser(user.username)}
                      >
                        Delete
                      </button>
                    )}
                  </>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {modalPlayer && (
        <PlayerStatsModal
          playerName={modalPlayer.name}
          playerId={modalPlayer.id}
          onClose={() => setModalPlayer(null)}
        />
      )}
    </div>
  )
}
