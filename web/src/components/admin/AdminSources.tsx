import { useCallback, useEffect, useState } from "react";
import { heartbeatHealth, healthLabel } from "../../utils/sourceHealth";
import type { PendingRequest } from "../../types";
import { UserPicker, type UserOption } from "./UserPicker";
import { apiFetch } from "../../authFetch";

type ApprovedSourceServer = {
  id: number;
  local_id: number;
  key: string;
  address: string;
  active: boolean;
};

type ApprovedSource = {
  source: string;
  version?: string;
  demo_base_url?: string;
  last_heartbeat_at?: string;
  is_remote: boolean;
  active: boolean;
  owner_user_id?: number;
  owner_username?: string;
  servers: ApprovedSourceServer[];
};

const SOURCE_NAME_PATTERN = /^[A-Za-z0-9_-]+$/;

export function AdminSources() {
  const [approved, setApproved] = useState<ApprovedSource[]>([]);
  const [pending, setPending] = useState<PendingRequest[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [newSource, setNewSource] = useState("");
  const [selectedOwner, setSelectedOwner] = useState<UserOption | null>(null);
  const [creating, setCreating] = useState(false);
  // The source whose transfer form is open. The picked user lives beside
  // it so closing the form drops an unsubmitted choice.
  const [transferTarget, setTransferTarget] = useState<string | null>(null);
  const [transferTo, setTransferTo] = useState<UserOption | null>(null);

  const newSourceTrimmed = newSource.trim();
  const newSourceValid =
    newSourceTrimmed !== "" && SOURCE_NAME_PATTERN.test(newSourceTrimmed);
  const formValid = newSourceValid && selectedOwner !== null;

  const fetchAll = useCallback(async () => {
    try {
      const res = await apiFetch("/api/admin/sources");
      if (!res.ok) throw new Error(`sources: ${res.status}`);
      setApproved((await res.json()) ?? []);
      setError("");
    } catch (e) {
      setError(`Couldn't load sources: ${(e as Error).message}`);
    } finally {
      setLoading(false);
    }
  }, []);

  const fetchPending = useCallback(async () => {
    try {
      const res = await apiFetch("/api/admin/sources/pending");
      if (!res.ok) throw new Error(`pending: ${res.status}`);
      setPending((await res.json()) ?? []);
    } catch (e) {
      setError(`Couldn't load pending requests: ${(e as Error).message}`);
    }
  }, []);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    fetchAll();
    fetchPending();
    const id = setInterval(() => {
      fetchAll();
      fetchPending();
    }, 15_000);
    return () => clearInterval(id);
  }, [fetchAll, fetchPending]);

  useEffect(() => {
    let cancelled = false;
    apiFetch("/api/account/profile")
      .then((res) => (res.ok ? res.json() : null))
      .then((profile) => {
        if (cancelled || !profile?.user) return;
        setSelectedOwner(
          (prev) =>
            prev ?? {
              id: profile.user.id,
              username: profile.user.username,
              is_admin: profile.user.is_admin,
              player_name: profile.player?.name ?? null,
            },
        );
      })
      .catch(() => {
        /* the picker starts empty */
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const approveRequest = async (req: PendingRequest) => {
    setError("");
    setNotice("");
    try {
      const res = await apiFetch(
        `/api/admin/sources/${encodeURIComponent(req.source)}/approve`,
        {
          method: "POST",
        },
      );
      if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
      setNotice(`Approved ${req.source}.`);
      await Promise.all([fetchPending(), fetchAll()]);
    } catch (err) {
      setError(`Couldn't approve ${req.source}: ${(err as Error).message}`);
    }
  };

  const rejectRequest = async (req: PendingRequest, reason: string) => {
    setError("");
    setNotice("");
    try {
      const res = await apiFetch(
        `/api/admin/sources/${encodeURIComponent(req.source)}/reject`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ reason }),
        },
      );
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || `${res.status} ${res.statusText}`);
      }
      setNotice(`Rejected ${req.source}.`);
      await fetchPending();
    } catch (err) {
      setError(`Couldn't reject ${req.source}: ${(err as Error).message}`);
    }
  };

  const renameRequest = async (
    req: PendingRequest,
    newName: string,
  ): Promise<string | null> => {
    setError("");
    setNotice("");
    try {
      const res = await apiFetch(
        `/api/admin/sources/${encodeURIComponent(req.source)}/rename`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ name: newName }),
        },
      );
      if (!res.ok) {
        const text = await res.text();
        return text || `${res.status} ${res.statusText}`;
      }
      setNotice(`Renamed ${req.source} → ${newName}.`);
      await fetchPending();
      return null;
    } catch (err) {
      return (err as Error).message;
    }
  };

  const downloadCredsBlob = async (res: Response, source: string) => {
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `${source}.creds`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  };

  const createSource = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setNotice("");
    if (!newSourceValid) {
      setError("Source names can use only letters, digits, - and _.");
      return;
    }
    if (!selectedOwner) {
      setError("Pick an owner. Every remote source needs one.");
      return;
    }
    setCreating(true);
    try {
      const res = await apiFetch("/api/admin/sources", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          source: newSourceTrimmed,
          owner_user_id: selectedOwner.id,
        }),
      });
      if (!res.ok) {
        throw new Error(`${res.status} ${res.statusText}`);
      }
      setNotice(`Created ${newSourceTrimmed}.`);
      setNewSource("");
      await fetchAll();
    } catch (err) {
      setError(
        `Couldn't create ${newSourceTrimmed}: ${(err as Error).message}`,
      );
    } finally {
      setCreating(false);
    }
  };

  const deactivateSource = async (source: ApprovedSource) => {
    if (
      !confirm(
        `Deactivate ${source.source}? Its credentials are revoked right away and its collector is cut off. Past matches stay, with its servers shown as inactive, and the owner can't rejoin under this name.`,
      )
    ) {
      return;
    }
    setError("");
    setNotice("");
    try {
      const res = await apiFetch(
        `/api/admin/sources/${encodeURIComponent(source.source)}/deactivate`,
        {
          method: "POST",
        },
      );
      if (!res.ok) {
        throw new Error(`${res.status} ${res.statusText}`);
      }
      setNotice(`Deactivated ${source.source}.`);
      await fetchAll();
    } catch (err) {
      setError(
        `Couldn't deactivate ${source.source}: ${(err as Error).message}`,
      );
    }
  };

  const transferOwner = async (
    source: ApprovedSource,
    newOwner: UserOption,
  ) => {
    setError("");
    setNotice("");
    try {
      const res = await apiFetch(
        `/api/admin/sources/${encodeURIComponent(source.source)}/owner`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ owner_user_id: newOwner.id }),
        },
      );
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || `${res.status} ${res.statusText}`);
      }
      setNotice(`Transferred ${source.source} to ${newOwner.username}.`);
      setTransferTarget(null);
      setTransferTo(null);
      await fetchAll();
    } catch (err) {
      setError(`Couldn't transfer ${source.source}: ${(err as Error).message}`);
    }
  };

  const reactivateSource = async (source: ApprovedSource) => {
    if (
      !confirm(
        `Reactivate ${source.source}? Its servers come back on the network with new credentials, so its collector needs the new .creds file.`,
      )
    ) {
      return;
    }
    setError("");
    setNotice("");
    try {
      const res = await apiFetch(
        `/api/admin/sources/${encodeURIComponent(source.source)}/reactivate`,
        {
          method: "POST",
        },
      );
      if (!res.ok) {
        throw new Error(`${res.status} ${res.statusText}`);
      }
      setNotice(
        `Reactivated ${source.source}. Download its new .creds file for the collector.`,
      );
      await fetchAll();
    } catch (err) {
      setError(
        `Couldn't reactivate ${source.source}: ${(err as Error).message}`,
      );
    }
  };

  const rotateCreds = async (source: ApprovedSource) => {
    if (
      !confirm(
        `Rotate the credentials for ${source.source}? Its collector needs the new .creds file to reconnect.`,
      )
    ) {
      return;
    }
    setError("");
    setNotice("");
    try {
      const res = await apiFetch(
        `/api/admin/sources/${encodeURIComponent(source.source)}/rotate-creds`,
        {
          method: "POST",
        },
      );
      if (!res.ok) {
        throw new Error(`${res.status} ${res.statusText}`);
      }
      await downloadCredsBlob(res, source.source);
      setNotice(
        `Rotated the credentials for ${source.source} and downloaded the new .creds file.`,
      );
    } catch (err) {
      setError(
        `Couldn't rotate the credentials for ${source.source}: ${(err as Error).message}`,
      );
    }
  };

  const downloadExisting = async (source: ApprovedSource) => {
    setError("");
    try {
      const res = await apiFetch(
        `/api/admin/sources/${encodeURIComponent(source.source)}/creds`,
        {},
      );
      if (!res.ok) {
        throw new Error(`${res.status} ${res.statusText}`);
      }
      await downloadCredsBlob(res, source.source);
    } catch (err) {
      setError(
        `Couldn't download the credentials for ${source.source}: ${(err as Error).message}`,
      );
    }
  };

  if (loading) {
    return <div>Loading sources…</div>;
  }

  return (
    <div className="admin-sources">
      {error && <div className="error-message">{error}</div>}
      {notice && <div className="notice-message">{notice}</div>}

      {pending.length > 0 && (
        <section className="admin-pending-requests">
          <h2>Pending requests ({pending.length})</h2>
          <p className="admin-help">
            Requests for a collector source. Approving creates its credentials
            and activates it. When you reject one, the requester sees your
            reason in My servers.
          </p>
          <table className="admin-data-table admin-table">
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

      <section className="admin-sources-create admin-form-card">
        <h2>Provision a new collector</h2>
        <p className="admin-help">
          Pick a short name. It scopes the source&rsquo;s NATS subjects, and the
          remote operator sets it as <code>tracker.collector.source_id</code>.
          Every remote source has an owner, who sees it under My servers. The
          owner defaults to you; pick another user to create it for them.
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
              selected={selectedOwner}
              onChange={setSelectedOwner}
              placeholder="Search users by name or linked player…"
              required
            />
            <p className="form-hint">
              The owner can download and rotate its credentials, take it off the
              network, and use RCON on its servers.
            </p>
          </div>
          <div className="admin-sources-form-actions">
            <button
              type="submit"
              className="admin-btn-primary"
              disabled={creating || !formValid}
            >
              {creating ? "Creating…" : "Create source"}
            </button>
          </div>
        </form>
      </section>

      <section>
        <h2>
          Provisioned sources
          <span className="admin-section-header__count">
            {" "}
            ({approved.length})
          </span>
        </h2>
        {approved.length === 0 ? (
          <p className="admin-empty">No sources yet.</p>
        ) : (
          <table className="admin-data-table admin-table">
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
                const h = heartbeatHealth(s.last_heartbeat_at);
                return (
                  <tr key={s.source}>
                    <td data-label="Health">
                      <span className={`health-badge health-${h}`}>
                        {healthLabel(h)}
                      </span>
                    </td>
                    <td data-label="Source">
                      <div className="admin-sources-source-name">
                        {s.source || (s.is_remote ? "—" : "local")}
                      </div>
                      {s.demo_base_url ? (
                        <div
                          className="admin-sources-demo-url"
                          title="The demo URL this collector reports"
                        >
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
                              e.preventDefault();
                              if (
                                transferTo &&
                                transferTo.id !== s.owner_user_id
                              ) {
                                transferOwner(s, transferTo);
                              }
                            }}
                          >
                            <UserPicker
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
                              disabled={
                                !transferTo || transferTo.id === s.owner_user_id
                              }
                            >
                              Save
                            </button>
                            <button
                              type="button"
                              className="admin-btn admin-btn-small admin-btn-ghost"
                              onClick={() => {
                                setTransferTarget(null);
                                setTransferTo(null);
                              }}
                            >
                              Cancel
                            </button>
                          </form>
                        ) : (
                          <div className="admin-sources-owner">
                            <span>
                              {s.owner_username || (
                                <span className="admin-muted">unassigned</span>
                              )}
                            </span>
                            <button
                              type="button"
                              className="admin-btn admin-btn-small admin-btn-ghost"
                              onClick={() => {
                                setTransferTarget(s.source);
                                setTransferTo(null);
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
                    <td data-label="Version">{s.version || "—"}</td>
                    <td data-label="Last heartbeat">
                      {s.last_heartbeat_at ?? "—"}
                    </td>
                    <td data-label="Servers">
                      {s.servers.length === 0 ? (
                        <span className="admin-muted">
                          awaiting first registration
                        </span>
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
                            <button
                              className="admin-btn"
                              onClick={() => downloadExisting(s)}
                            >
                              Download creds
                            </button>
                            <button
                              className="admin-btn admin-btn-warn"
                              onClick={() => rotateCreds(s)}
                            >
                              Rotate creds
                            </button>
                            <button
                              className="admin-btn admin-btn-danger"
                              onClick={() => deactivateSource(s)}
                            >
                              Deactivate
                            </button>
                          </>
                        ) : (
                          <button
                            className="admin-btn"
                            onClick={() => reactivateSource(s)}
                          >
                            Reactivate
                          </button>
                        )
                      ) : (
                        <span className="admin-muted">hub-local</span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </section>
    </div>
  );
}

// Rename errors show in the row; approve and reject errors use the
// page banner.
interface PendingRowProps {
  req: PendingRequest;
  onApprove: () => void;
  onReject: (reason: string) => void;
  onRename: (newName: string) => Promise<string | null>;
}

const NAME_PATTERN = /^[A-Za-z0-9_-]{3,16}$/;

function PendingRow({ req, onApprove, onReject, onRename }: PendingRowProps) {
  const [mode, setMode] = useState<"idle" | "rejecting" | "renaming">("idle");
  const [text, setText] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [renameError, setRenameError] = useState<string | null>(null);

  const reset = () => {
    setMode("idle");
    setText("");
    setRenameError(null);
  };

  const submitReject = async () => {
    if (!text.trim()) return;
    setSubmitting(true);
    await onReject(text.trim());
    setSubmitting(false);
    reset();
  };

  const submitRename = async () => {
    setRenameError(null);
    if (!NAME_PATTERN.test(text)) {
      setRenameError("Name must be 3–16 characters: letters, numbers, _ or -.");
      return;
    }
    setSubmitting(true);
    const err = await onRename(text);
    setSubmitting(false);
    if (err) {
      setRenameError(err);
      return;
    }
    reset();
  };

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
        {mode === "rejecting" && (
          <div className="admin-pending-inline-form">
            <input
              type="text"
              value={text}
              onChange={(e) => setText(e.target.value)}
              placeholder="Reason (the requester sees this)"
              autoFocus
            />
            <button
              className="admin-btn admin-btn-danger"
              onClick={submitReject}
              disabled={submitting || !text.trim()}
            >
              {submitting ? "…" : "Confirm reject"}
            </button>
            <button className="admin-btn" onClick={reset} disabled={submitting}>
              Cancel
            </button>
          </div>
        )}
        {mode === "renaming" && (
          <div className="admin-pending-inline-form">
            <input
              type="text"
              value={text}
              onChange={(e) => setText(e.target.value)}
              placeholder="New name, such as mygame-jfk"
              autoFocus
            />
            <button
              className="admin-btn"
              onClick={submitRename}
              disabled={submitting || !text.trim()}
            >
              {submitting ? "…" : "Confirm rename"}
            </button>
            <button className="admin-btn" onClick={reset} disabled={submitting}>
              Cancel
            </button>
            {renameError && <div className="error-message">{renameError}</div>}
          </div>
        )}
        {mode === "idle" && (
          <>
            <button className="admin-btn-primary" onClick={onApprove}>
              Approve
            </button>
            <button
              className="admin-btn"
              onClick={() => {
                setMode("renaming");
                setText(req.source);
              }}
            >
              Rename
            </button>
            <button
              className="admin-btn-danger"
              onClick={() => setMode("rejecting")}
            >
              Reject
            </button>
          </>
        )}
      </td>
    </tr>
  );
}
