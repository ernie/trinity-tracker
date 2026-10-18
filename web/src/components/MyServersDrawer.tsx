import { useState, type FormEvent } from "react";
import { apiFetch } from "../authFetch";
import { heartbeatHealth, healthLabel, timeAgo } from "../utils/sourceHealth";
import { StatusDot } from "./StatusDot";
import { CloseIcon } from "./CloseIcon";
import type { MySourceEntry } from "../types";

export interface MyServersDrawerProps {
  sources: MySourceEntry[];
  hasPending: boolean;
  onClose: () => void;
  onRefresh: () => void;
}

export function MyServersDrawer({
  sources,
  hasPending,
  onClose,
  onRefresh,
}: MyServersDrawerProps) {
  const isEmpty = sources.length === 0;
  const lastSetback = sources.find(
    (s) => s.status === "rejected" || s.status === "left",
  );
  const formInitial = lastSetback
    ? { name: lastSetback.source, purpose: lastSetback.purpose ?? "" }
    : undefined;
  const rejectionReason =
    lastSetback?.status === "rejected"
      ? lastSetback.rejection_reason
      : undefined;

  const heading = isEmpty
    ? "Add servers"
    : hasPending
      ? "Request pending"
      : "My servers";

  return (
    <div className="drawer-overlay" onClick={onClose}>
      <aside
        className="drawer my-servers-drawer"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="drawer-header">
          <h2>{heading}</h2>
          <button className="close-btn" onClick={onClose} aria-label="Close">
            <CloseIcon />
          </button>
        </div>
        <div className="drawer-body">
          {sources.map((s) => (
            <SourceCard key={s.source} src={s} onUpdated={onRefresh} />
          ))}
          {hasPending ? (
            <p className="drawer-add-another muted">
              You can submit another request once your pending one is reviewed.
            </p>
          ) : (
            <RequestForm
              startExpanded={isEmpty}
              showExplainer={isEmpty}
              initial={formInitial}
              rejectionReason={rejectionReason}
              onSubmitted={onRefresh}
            />
          )}
        </div>
      </aside>
    </div>
  );
}

interface SourceCardProps {
  src: MySourceEntry;
  onUpdated: () => void;
}

function SourceCard({ src, onUpdated }: SourceCardProps) {
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function downloadCreds() {
    setBusy("download");
    setError(null);
    try {
      const r = await apiFetch(
        `/api/sources/mine/${encodeURIComponent(src.source)}/creds`,
      );
      if (!r.ok) throw new Error((await r.text()) || `HTTP ${r.status}`);
      saveBlob(await r.blob(), `${src.source}.creds`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "download failed");
    } finally {
      setBusy(null);
    }
  }

  async function rotateCreds() {
    if (
      !confirm(
        `Rotate credentials for ${src.source}? The current .creds file will stop working immediately.`,
      )
    ) {
      return;
    }
    setBusy("rotate");
    setError(null);
    try {
      const r = await apiFetch(
        `/api/sources/mine/${encodeURIComponent(src.source)}/rotate-creds`,
        { method: "POST" },
      );
      if (!r.ok) throw new Error((await r.text()) || `HTTP ${r.status}`);
      saveBlob(await r.blob(), `${src.source}.creds`);
      onUpdated();
    } catch (e) {
      setError(e instanceof Error ? e.message : "rotate failed");
    } finally {
      setBusy(null);
    }
  }

  async function leave() {
    if (
      !confirm(
        `Leave the network with ${src.source}? Its servers come off the network and their match history stays. You can rejoin anytime, and your other sources aren't affected.`,
      )
    ) {
      return;
    }
    setBusy("leave");
    setError(null);
    try {
      const r = await apiFetch(
        `/api/sources/mine/${encodeURIComponent(src.source)}/leave`,
        { method: "POST" },
      );
      if (!r.ok) throw new Error((await r.text()) || `HTTP ${r.status}`);
      onUpdated();
    } catch (e) {
      setError(e instanceof Error ? e.message : "leave failed");
      setBusy(null);
    }
  }

  if (src.status === "pending") {
    return (
      <section className="source-card source-card-pending">
        <header className="source-card-header">
          <strong>{src.source}</strong>
          <span className="status-pill status-pending">Pending review</span>
        </header>
        <p className="muted">An admin will review shortly.</p>
        {src.purpose && (
          <p>
            <em>Purpose:</em> {src.purpose}
          </p>
        )}
      </section>
    );
  }
  if (src.status === "rejected") {
    return (
      <section className="source-card source-card-rejected">
        <header className="source-card-header">
          <strong>{src.source}</strong>
          <span className="status-pill status-rejected">Rejected</span>
        </header>
        {src.rejection_reason && (
          <p className="error-message">
            <strong>Reason:</strong> {src.rejection_reason}
          </p>
        )}
      </section>
    );
  }
  if (src.status === "left") {
    return (
      <section className="source-card source-card-left">
        <header className="source-card-header">
          <strong>{src.source}</strong>
          <span className="status-pill status-left">Left network</span>
        </header>
        <p className="muted">
          You took this source off the network. Request this name again to
          rejoin; it&rsquo;s approved automatically.
        </p>
      </section>
    );
  }
  if (src.status === "revoked") {
    return (
      <section className="source-card source-card-revoked">
        <header className="source-card-header">
          <strong>{src.source}</strong>
          <span className="status-pill status-revoked">Revoked by admin</span>
        </header>
        <p className="muted">Contact a hub admin to re-enable this source.</p>
      </section>
    );
  }

  const health = heartbeatHealth(src.last_heartbeat_at);
  return (
    <section className="source-card source-card-active">
      <header className="source-card-header">
        <strong>{src.source}</strong>
        <span className={`health-badge health-${health}`}>
          {healthLabel(health)}
        </span>
      </header>
      <div className="source-card-meta">
        {src.last_heartbeat_at && (
          <span>last heartbeat {timeAgo(src.last_heartbeat_at)}</span>
        )}
        {src.version && <span>· engine {src.version}</span>}
        {src.demo_base_url && (
          <span>
            ·{" "}
            <a href={src.demo_base_url} target="_blank" rel="noreferrer">
              {src.demo_base_url}
            </a>
          </span>
        )}
      </div>

      <div className="source-card-actions">
        <button onClick={downloadCreds} disabled={busy !== null}>
          {busy === "download" ? "Downloading…" : "Download .creds"}
        </button>
        <button className="warn" onClick={rotateCreds} disabled={busy !== null}>
          {busy === "rotate" ? "Rotating…" : "Rotate creds"}
        </button>
        <button className="danger" onClick={leave} disabled={busy !== null}>
          {busy === "leave" ? "Leaving…" : "Leave network"}
        </button>
      </div>
      {error && <div className="error-message">{error}</div>}

      <ul className="source-card-server-list">
        {(src.servers ?? []).map((s) => {
          // s.active means registered; only a heartbeating source's servers
          // count as running.
          const live = s.active && health === "green";
          return (
            <li key={s.key} className={live ? "" : "inactive"}>
              <span className="server-key">{s.key}</span>
              <span className="server-addr">{s.address}</span>
              <span className={`server-state ${live ? "running" : "idle"}`}>
                <StatusDot active={live} /> {live ? "running" : "idle"}
              </span>
            </li>
          );
        })}
        {(src.servers ?? []).length === 0 && (
          <li className="muted">
            No servers registered yet. They appear once your collector starts.
          </li>
        )}
      </ul>
    </section>
  );
}

// Matches the hub's source name validation.
const NAME_PATTERN = /^[A-Za-z0-9_-]{3,16}$/;
const MAX_PURPOSE_LEN = 200;

interface RequestFormProps {
  startExpanded: boolean;
  showExplainer: boolean;
  initial?: { name: string; purpose: string };
  rejectionReason?: string;
  onSubmitted: () => void;
}

function RequestForm({
  startExpanded,
  showExplainer,
  initial,
  rejectionReason,
  onSubmitted,
}: RequestFormProps) {
  const [expanded, setExpanded] = useState(startExpanded);
  const [name, setName] = useState(initial?.name ?? "");
  const [purpose, setPurpose] = useState(initial?.purpose ?? "");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    if (!NAME_PATTERN.test(name)) {
      setError("Name must be 3–16 characters: letters, numbers, _ or -.");
      return;
    }
    if (purpose.length > MAX_PURPOSE_LEN) {
      setError(`Purpose can be at most ${MAX_PURPOSE_LEN} characters.`);
      return;
    }
    setSubmitting(true);
    try {
      const r = await apiFetch("/api/sources/request", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, purpose }),
      });
      if (!r.ok) {
        const text = await r.text();
        let message = text;
        try {
          const parsed = JSON.parse(text) as { error?: string };
          if (parsed.error) message = parsed.error;
        } catch {
          // not JSON, use raw text
        }
        setError(message || `HTTP ${r.status}`);
        setSubmitting(false);
        return;
      }
      setName("");
      setPurpose("");
      setExpanded(startExpanded);
      onSubmitted();
    } catch (e) {
      setError(e instanceof Error ? e.message : "submission failed");
    } finally {
      setSubmitting(false);
    }
  }

  if (!expanded) {
    return (
      <div className="drawer-add-another">
        <button onClick={() => setExpanded(true)}>+ Add another source</button>
      </div>
    );
  }

  return (
    <section className="request-form">
      {showExplainer && (
        <div className="drawer-help">
          <p>
            A <strong>source</strong> is one machine, physical or virtual, that
            hosts one or more Quake III Arena servers and runs the Trinity
            collector. Submit one request per machine, since each gets its own
            credential and its own rotate and leave controls.
          </p>
          <p>
            The host needs the Trinity engine and collector. Once an admin
            approves your request, download the .creds file here; the collector
            loads it when it starts.
          </p>
        </div>
      )}
      {rejectionReason && (
        <div className="error-message" role="alert">
          <strong>Previously rejected:</strong> {rejectionReason}
        </div>
      )}
      <form onSubmit={handleSubmit}>
        <div className="form-group">
          <label>Source name</label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="mygame-jfk"
            disabled={submitting}
            minLength={3}
            maxLength={16}
            autoFocus
            required
          />
          <small>
            3–16 characters: letters, numbers, _ or -. If you run hosts in
            several regions, a name plus a location code, such as{" "}
            <code>mygame-jfk</code> or <code>mygame-fra</code>, works well.
          </small>
        </div>
        <div className="form-group">
          <label>
            What is this for? <small>(optional)</small>
          </label>
          <textarea
            value={purpose}
            onChange={(e) => setPurpose(e.target.value)}
            maxLength={MAX_PURPOSE_LEN}
            rows={3}
            disabled={submitting}
          />
          <small>
            {purpose.length}/{MAX_PURPOSE_LEN}
          </small>
        </div>
        {error && <div className="error-message">{error}</div>}
        <div className="request-form-actions">
          {!startExpanded && (
            <button
              type="button"
              className="cancel-btn"
              onClick={() => {
                setExpanded(false);
                setError(null);
              }}
              disabled={submitting}
            >
              Cancel
            </button>
          )}
          <button type="submit" disabled={submitting}>
            {submitting ? "Submitting…" : "Submit request"}
          </button>
        </div>
      </form>
    </section>
  );
}

function saveBlob(blob: Blob, name: string) {
  const u = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = u;
  a.download = name;
  a.click();
  URL.revokeObjectURL(u);
}
