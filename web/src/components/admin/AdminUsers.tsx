import { useState, useEffect, FormEvent } from "react";
import { useAuth } from "../../hooks/useAuth";
import { useDebouncedValue } from "../../hooks/useDebouncedValue";
import { ColoredText } from "../ColoredText";
import { DisplayNameEditor } from "../DisplayNameEditor";
import { PlayerStatsModal } from "../PlayerStatsModal";
import type { User, PlayerProfile } from "../../types";
import { apiFetch } from "../../authFetch";
import { cleanQ3DisplayName, canonicalizeDisplayName } from "../../utils";

export function AdminUsers() {
  const { auth } = useAuth();
  const currentUsername = auth.username!;

  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [createUsername, setCreateUsername] = useState("");
  const [createPassword, setCreatePassword] = useState("");
  const [createIsAdmin, setCreateIsAdmin] = useState(false);
  const [createPlayerId, setCreatePlayerId] = useState<number | null>(null);
  const [resetPasswordUserId, setResetPasswordUserId] = useState<number | null>(
    null,
  );
  const [newPassword, setNewPassword] = useState("");
  const [playerSearch, setPlayerSearch] = useState("");
  const [playerResults, setPlayerResults] = useState<PlayerProfile[]>([]);
  const [editingUserId, setEditingUserId] = useState<number | null>(null);
  const [editPlayerSearch, setEditPlayerSearch] = useState("");
  const [editPlayerResults, setEditPlayerResults] = useState<PlayerProfile[]>(
    [],
  );
  const [editPlayerId, setEditPlayerId] = useState<number | null>(null);
  const [editingNameUserId, setEditingNameUserId] = useState<number | null>(
    null,
  );
  const [editNameValue, setEditNameValue] = useState("");
  const [modalPlayer, setModalPlayer] = useState<{
    id: number;
    name: string;
  } | null>(null);

  const fetchUsers = async () => {
    try {
      const res = await apiFetch("/api/users");
      if (res.ok) {
        setUsers(await res.json());
      } else {
        setError("Couldn't load users.");
      }
    } catch {
      setError("Couldn't load users.");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    fetchUsers();
  }, []);

  const handleCreateUser = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    setNotice("");

    if (createPassword.length < 8) {
      setError("Password must be at least 8 characters.");
      return;
    }

    try {
      const res = await apiFetch("/api/users", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          username: createUsername,
          password: createPassword,
          is_admin: createIsAdmin,
          player_id: createPlayerId,
        }),
      });

      if (res.ok) {
        const data = await res.json();
        setNotice(data.message);
        setShowCreateForm(false);
        setCreateUsername("");
        setCreatePassword("");
        setCreateIsAdmin(false);
        setCreatePlayerId(null);
        setPlayerSearch("");
        fetchUsers();
      } else {
        const data = await res.json();
        setError(data.error || "Couldn't create the user.");
      }
    } catch {
      setError("Couldn't reach the server.");
    }
  };

  const handleDeleteUser = async (username: string) => {
    if (!confirm(`Delete user "${username}"?`)) return;
    setError("");
    setNotice("");

    try {
      const res = await apiFetch(`/api/users/${username}`, {
        method: "DELETE",
      });

      if (res.ok) {
        const data = await res.json();
        setNotice(data.message);
        fetchUsers();
      } else {
        const data = await res.json();
        setError(data.error || "Couldn't delete the user.");
      }
    } catch {
      setError("Couldn't reach the server.");
    }
  };

  const handleResetPassword = async (userId: number) => {
    setError("");
    setNotice("");
    if (newPassword.length < 8) {
      setError("Password must be at least 8 characters.");
      return;
    }

    try {
      const res = await apiFetch(`/api/users/${userId}/reset-password`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ new_password: newPassword }),
      });

      if (res.ok) {
        const data = await res.json();
        setNotice(data.message);
        setResetPasswordUserId(null);
        setNewPassword("");
        fetchUsers();
      } else {
        const data = await res.json();
        setError(data.error || "Couldn't reset the password.");
      }
    } catch {
      setError("Couldn't reach the server.");
    }
  };

  const debouncedPlayerSearch = useDebouncedValue(playerSearch, 200);
  const debouncedEditPlayerSearch = useDebouncedValue(editPlayerSearch, 200);

  useEffect(() => {
    if (debouncedPlayerSearch.length < 2) return;
    const ctrl = new AbortController();
    apiFetch(
      `/api/players?search=${encodeURIComponent(debouncedPlayerSearch)}&limit=10`,
      {
        signal: ctrl.signal,
      },
    )
      .then((res) => (res.ok ? res.json() : []))
      .then((players: PlayerProfile[]) => setPlayerResults(players ?? []))
      .catch(() => {
        /* aborted or network error */
      });
    return () => ctrl.abort();
  }, [debouncedPlayerSearch]);

  useEffect(() => {
    if (debouncedEditPlayerSearch.length < 2) return;
    const ctrl = new AbortController();
    apiFetch(
      `/api/players?search=${encodeURIComponent(debouncedEditPlayerSearch)}&limit=10`,
      {
        signal: ctrl.signal,
      },
    )
      .then((res) => (res.ok ? res.json() : []))
      .then((players: PlayerProfile[]) => setEditPlayerResults(players ?? []))
      .catch(() => {
        /* aborted or network error */
      });
    return () => ctrl.abort();
  }, [debouncedEditPlayerSearch]);

  const displayPlayerResults =
    debouncedPlayerSearch.length >= 2 ? playerResults : [];
  const displayEditPlayerResults =
    debouncedEditPlayerSearch.length >= 2 ? editPlayerResults : [];

  const startEditingUser = (user: User) => {
    setEditingUserId(user.id);
    setEditPlayerId(user.player_id);
    setEditPlayerSearch("");
    setEditPlayerResults([]);
  };

  const cancelEditing = () => {
    setEditingUserId(null);
    setEditPlayerId(null);
    setEditPlayerSearch("");
    setEditPlayerResults([]);
  };

  const startEditingName = (user: User) => {
    setEditingNameUserId(user.id);
    setEditNameValue(user.display_name);
  };

  const cancelEditingName = () => {
    setEditingNameUserId(null);
    setEditNameValue("");
  };

  const handleUpdateDisplayName = async (userId: number) => {
    const cleaned = cleanQ3DisplayName(editNameValue);
    if (!cleaned || !canonicalizeDisplayName(cleaned)) return;
    setError("");
    try {
      const res = await apiFetch(`/api/users/${userId}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ display_name: cleaned }),
      });

      if (res.ok) {
        cancelEditingName();
        fetchUsers();
      } else {
        const data = await res.json();
        setError(data.error || "Couldn't update the display name.");
      }
    } catch {
      setError("Couldn't reach the server.");
    }
  };

  const handleUpdatePlayerLink = async (userId: number) => {
    setError("");
    setNotice("");
    try {
      const res = await apiFetch(`/api/users/${userId}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ player_id: editPlayerId }),
      });

      if (res.ok) {
        const data = await res.json();
        setNotice(data.message);
        cancelEditing();
        fetchUsers();
      } else {
        const data = await res.json();
        setError(data.error || "Couldn't update the user.");
      }
    } catch {
      setError("Couldn't reach the server.");
    }
  };

  if (loading) {
    return <div className="admin-loading">Loading users…</div>;
  }

  return (
    <div className="admin-users">
      <div className="admin-section-header">
        <h2>
          Users
          <span className="admin-section-header__count"> ({users.length})</span>
        </h2>
      </div>

      {error && <div className="error-message">{error}</div>}
      {notice && <div className="notice-message">{notice}</div>}

      <details
        open={showCreateForm}
        onToggle={(e) =>
          setShowCreateForm((e.target as HTMLDetailsElement).open)
        }
      >
        <summary className="admin-form-toggle">
          <span className="admin-form-toggle__caret" aria-hidden="true">
            ▸
          </span>
          {showCreateForm ? "Cancel" : "Create user"}
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
            <label>Temporary password</label>
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
            <label>Link to a player (optional)</label>
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
                      setCreatePlayerId(p.id);
                      setPlayerSearch(p.clean_name);
                      setPlayerResults([]);
                    }}
                  >
                    {p.clean_name}
                  </li>
                ))}
              </ul>
            )}
            {createPlayerId && (
              <div className="selected-player">
                <span>Player #{createPlayerId}</span>
                <button
                  type="button"
                  onClick={() => {
                    setCreatePlayerId(null);
                    setPlayerSearch("");
                  }}
                >
                  Clear
                </button>
              </div>
            )}
          </div>
          <button type="submit" className="admin-btn-primary">
            Create user
          </button>
        </form>
      </details>

      <table className="admin-data-table users-table">
        <thead>
          <tr>
            <th>Username</th>
            <th>Name</th>
            <th>Role</th>
            <th>Player</th>
            <th>Must change password</th>
            <th>Last login</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {users.map((user) => (
            <tr key={user.id}>
              <td data-label="Username">{user.username}</td>
              <td data-label="Name">
                {editingNameUserId === user.id ? (
                  <div className="edit-player-inline">
                    <DisplayNameEditor
                      value={editNameValue}
                      onChange={setEditNameValue}
                      mode="claim"
                      onSubmit={() => handleUpdateDisplayName(user.id)}
                    />
                    <div className="edit-actions">
                      <button
                        className="admin-btn-primary"
                        disabled={
                          !canonicalizeDisplayName(
                            cleanQ3DisplayName(editNameValue),
                          )
                        }
                        onClick={() => handleUpdateDisplayName(user.id)}
                      >
                        Save
                      </button>
                      <button className="admin-btn" onClick={cancelEditingName}>
                        Cancel
                      </button>
                    </div>
                  </div>
                ) : (
                  <span className="player-link-display">
                    {user.display_name ? (
                      <ColoredText text={user.display_name} />
                    ) : (
                      <span className="admin-muted">—</span>
                    )}
                    <button
                      className="edit-link-btn"
                      onClick={() => startEditingName(user)}
                    >
                      Edit
                    </button>
                  </span>
                )}
              </td>
              <td data-label="Role">{user.is_admin ? "Admin" : "User"}</td>
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
                              setEditPlayerId(p.id);
                              setEditPlayerSearch(p.clean_name);
                              setEditPlayerResults([]);
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
                            setEditPlayerId(null);
                            setEditPlayerSearch("");
                          }}
                        >
                          Clear
                        </button>
                      </div>
                    )}
                    <div className="edit-actions">
                      <button
                        className="admin-btn-primary"
                        onClick={() => handleUpdatePlayerLink(user.id)}
                      >
                        Save
                      </button>
                      <button className="admin-btn" onClick={cancelEditing}>
                        Cancel
                      </button>
                    </div>
                  </div>
                ) : (
                  <span className="player-link-display">
                    {user.player_id && user.player_name ? (
                      <button
                        type="button"
                        className="player-name-link"
                        onClick={() =>
                          setModalPlayer({
                            id: user.player_id!,
                            name: user.player_name!,
                          })
                        }
                      >
                        <ColoredText text={user.player_name} />
                      </button>
                    ) : (
                      <span className="admin-muted">—</span>
                    )}
                    <button
                      className="edit-link-btn"
                      onClick={() => startEditingUser(user)}
                    >
                      Edit
                    </button>
                  </span>
                )}
              </td>
              <td data-label="Must change password">
                {user.password_change_required ? "Yes" : "No"}
              </td>
              <td data-label="Last login">
                {user.last_login
                  ? new Date(user.last_login).toLocaleDateString()
                  : "Never"}
              </td>
              <td data-label="Actions" className="actions">
                {resetPasswordUserId === user.id ? (
                  <div className="reset-password-inline">
                    <input
                      type="password"
                      placeholder="New password"
                      value={newPassword}
                      onChange={(e) => setNewPassword(e.target.value)}
                    />
                    <button
                      className="admin-btn-primary"
                      onClick={() => handleResetPassword(user.id)}
                    >
                      Save
                    </button>
                    <button
                      className="admin-btn"
                      onClick={() => {
                        setResetPasswordUserId(null);
                        setNewPassword("");
                      }}
                    >
                      Cancel
                    </button>
                  </div>
                ) : (
                  <>
                    <button
                      className="admin-btn"
                      onClick={() => setResetPasswordUserId(user.id)}
                    >
                      Reset password
                    </button>
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
  );
}
