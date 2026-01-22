import { useState, useEffect, FormEvent } from 'react'
import type { User, PlayerProfile } from '../types'

interface UserManagementProps {
  token: string
  currentUsername: string
  onClose: () => void
}

export function UserManagement({ token, currentUsername, onClose }: UserManagementProps) {
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
    fetchUsers()
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

  const searchPlayers = async (query: string, forEdit = false) => {
    if (query.length < 2) {
      if (forEdit) {
        setEditPlayerResults([])
      } else {
        setPlayerResults([])
      }
      return
    }

    try {
      const res = await fetch(`/api/players?search=${encodeURIComponent(query)}&limit=10`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (res.ok) {
        const players = await res.json()
        if (forEdit) {
          setEditPlayerResults(players)
        } else {
          setPlayerResults(players)
        }
      }
    } catch {
      // Ignore search errors
    }
  }

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
    return (
      <div className="modal-overlay">
        <div className="modal user-management-modal">
          <div className="modal-header">
            <h2>User Management</h2>
            <button className="close-btn" onClick={onClose}>&times;</button>
          </div>
          <div className="loading">Loading users...</div>
        </div>
      </div>
    )
  }

  return (
    <div className="modal-overlay">
      <div className="modal user-management-modal">
        <div className="modal-header">
          <h2>User Management</h2>
          <button className="close-btn" onClick={onClose}>&times;</button>
        </div>

        {error && <div className="error-message">{error}</div>}

        <div className="user-actions">
          <button onClick={() => setShowCreateForm(!showCreateForm)}>
            {showCreateForm ? 'Cancel' : 'Create User'}
          </button>
        </div>

        {showCreateForm && (
          <form className="create-user-form" onSubmit={handleCreateUser}>
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
                placeholder="Search players..."
                value={playerSearch}
                onChange={(e) => {
                  setPlayerSearch(e.target.value)
                  searchPlayers(e.target.value)
                }}
              />
              {playerResults.length > 0 && (
                <ul className="player-results">
                  {playerResults.map((p) => (
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
                  Selected: Player #{createPlayerId}
                  <button type="button" onClick={() => { setCreatePlayerId(null); setPlayerSearch('') }}>Clear</button>
                </div>
              )}
            </div>
            <button type="submit">Create User</button>
          </form>
        )}

        <table className="users-table">
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
                <td>{user.username}</td>
                <td>{user.is_admin ? 'Admin' : 'User'}</td>
                <td>
                  {editingUserId === user.id ? (
                    <div className="edit-player-inline">
                      <input
                        type="text"
                        placeholder="Search players..."
                        value={editPlayerSearch}
                        onChange={(e) => {
                          setEditPlayerSearch(e.target.value)
                          searchPlayers(e.target.value, true)
                        }}
                      />
                      {editPlayerResults.length > 0 && (
                        <ul className="player-results">
                          {editPlayerResults.map((p) => (
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
                          Player #{editPlayerId}
                          <button type="button" onClick={() => { setEditPlayerId(null); setEditPlayerSearch('') }}>Clear</button>
                        </div>
                      )}
                      <div className="edit-actions">
                        <button onClick={() => handleUpdatePlayerLink(user.id)}>Save</button>
                        <button onClick={cancelEditing}>Cancel</button>
                      </div>
                    </div>
                  ) : (
                    <span className="player-link-display">
                      {user.player_id || '-'}
                      <button className="edit-link-btn" onClick={() => startEditingUser(user)}>Edit</button>
                    </span>
                  )}
                </td>
                <td>{user.password_change_required ? 'Yes' : 'No'}</td>
                <td>{user.last_login ? new Date(user.last_login).toLocaleDateString() : 'Never'}</td>
                <td className="actions">
                  {resetPasswordUserId === user.id ? (
                    <div className="reset-password-inline">
                      <input
                        type="password"
                        placeholder="New password"
                        value={newPassword}
                        onChange={(e) => setNewPassword(e.target.value)}
                      />
                      <button onClick={() => handleResetPassword(user.id)}>Save</button>
                      <button onClick={() => {
                        setResetPasswordUserId(null)
                        setNewPassword('')
                      }}>Cancel</button>
                    </div>
                  ) : (
                    <>
                      <button onClick={() => setResetPasswordUserId(user.id)}>Reset Pwd</button>
                      {user.username !== currentUsername && (
                        <button className="delete-btn" onClick={() => handleDeleteUser(user.username)}>Delete</button>
                      )}
                    </>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
