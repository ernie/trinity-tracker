import { useState, useEffect, useCallback } from "react";
import { Link } from "react-router-dom";
import { apiFetch } from "../../authFetch";
import { ColoredText } from "../ColoredText";
import { formatDateTime, formatDuration } from "../../utils/formatters";
import type { AdminSession, Server } from "../../types";
import {
  AdminSessionsFilters,
  EMPTY_ADMIN_SESSIONS_FILTERS,
  loadAdminSessionsFilters,
  resolveDateRange,
  type AdminSessionsFilterState,
} from "./AdminSessionsFilters";

const PAGE_SIZE = 50;

export function AdminSessions() {
  const [servers, setServers] = useState<Server[]>([]);
  // Hydrate from localStorage on first render so admins keep their
  // server / player / date scope between visits.
  const [filters, setFilters] = useState<AdminSessionsFilterState>(
    loadAdminSessionsFilters,
  );

  const [sessions, setSessions] = useState<AdminSession[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [hasMore, setHasMore] = useState(false);

  // Load server list for the filter section's switches.
  useEffect(() => {
    apiFetch("/api/servers")
      .then((res) => (res.ok ? res.json() : []))
      .then((data) => setServers(data || []))
      .catch(() => setServers([]));
  }, []);

  const buildUrl = useCallback(
    (beforeID?: number) => {
      const params = new URLSearchParams();
      params.set("limit", String(PAGE_SIZE));
      // Server filter: backend takes a single server_id. The structured
      // panel uses a "hidden" set, so we infer the visible set as the
      // complement. When exactly one server remains visible, use it as
      // the backend filter; otherwise fall back to client-side filtering
      // of the returned page (rare path — admins typically pick zero or
      // one server, hiding the rest).
      const visible = servers.filter(
        (s) => !filters.hiddenServers.includes(s.id),
      );
      if (servers.length > 0 && visible.length === 1) {
        params.set("server_id", String(visible[0].id));
      }
      if (filters.playerId !== null) {
        params.set("player_id", String(filters.playerId));
      }
      const range = resolveDateRange(filters);
      if (range.since) params.set("since", range.since);
      if (range.until) params.set("until", range.until);
      if (beforeID) params.set("before", String(beforeID));
      return `/api/admin/sessions?${params.toString()}`;
    },
    [servers, filters],
  );

  // Client-side cleanup of the returned page when the visible-server
  // set has multiple values (we can't push that to the single-value
  // backend filter). Acts as a defensive layer; the backend has done
  // the heavy lifting already.
  const visibleServerSet = (() => {
    if (filters.hiddenServers.length === 0) return null;
    const visible = servers
      .filter((s) => !filters.hiddenServers.includes(s.id))
      .map((s) => s.id);
    return new Set(visible);
  })();

  const loadFirstPage = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const res = await apiFetch(buildUrl());
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || "Failed to load sessions");
      }
      const data: AdminSession[] = (await res.json()) || [];
      setSessions(data);
      setHasMore(data.length === PAGE_SIZE);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load sessions");
      setSessions([]);
      setHasMore(false);
    } finally {
      setLoading(false);
    }
  }, [buildUrl]);

  const loadMore = useCallback(async () => {
    if (loading || sessions.length === 0) return;
    const lastID = sessions[sessions.length - 1].id;
    setLoading(true);
    try {
      const res = await apiFetch(buildUrl(lastID));
      if (!res.ok) throw new Error("Failed to load more");
      const data: AdminSession[] = (await res.json()) || [];
      setSessions((prev) => [...prev, ...data]);
      setHasMore(data.length === PAGE_SIZE);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load more");
    } finally {
      setLoading(false);
    }
  }, [buildUrl, sessions, loading]);

  // Reload from page 1 whenever filters change.
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    loadFirstPage();
  }, [loadFirstPage]);

  const visibleSessions = visibleServerSet
    ? sessions.filter((s) => visibleServerSet.has(s.server_id))
    : sessions;

  const hasActive =
    filters.hiddenServers.length > 0 ||
    filters.playerId !== null ||
    filters.datePreset !== "";

  return (
    <div className="admin-sessions">
      <div className="admin-section-header">
        <h2>
          Player sessions
          <span className="admin-section-header__count">
            {" "}
            ({visibleSessions.length}
            {hasMore ? "+" : ""})
          </span>
        </h2>
        {hasActive && (
          <button
            type="button"
            className="admin-btn"
            onClick={() => setFilters(EMPTY_ADMIN_SESSIONS_FILTERS)}
          >
            Clear filters
          </button>
        )}
      </div>

      <AdminSessionsFilters
        servers={servers}
        state={filters}
        onChange={setFilters}
      />

      {error && <div className="error-message">{error}</div>}

      <table className="admin-data-table sessions-table">
        <thead>
          <tr>
            <th>Player</th>
            <th>Server</th>
            <th>Joined</th>
            <th>Duration</th>
            <th>IP</th>
            <th>Client</th>
          </tr>
        </thead>
        <tbody>
          {visibleSessions.map((s) => (
            <tr key={s.id}>
              <td data-label="Player">
                <Link to={`/players/${s.player_id}`}>
                  <ColoredText text={s.player_name} />
                </Link>
              </td>
              <td data-label="Server">
                {s.server_source} / {s.server_key}
              </td>
              <td data-label="Joined">{formatDateTime(s.joined_at)}</td>
              <td data-label="Duration">
                {s.duration_seconds ? (
                  formatDuration(s.duration_seconds)
                ) : s.left_at ? (
                  "—"
                ) : (
                  <em>active</em>
                )}
              </td>
              <td data-label="IP" className="ip-address">
                {s.ip_address || "—"}
              </td>
              <td data-label="Client">
                {s.client_engine
                  ? `${s.client_engine}${s.client_version ? ` ${s.client_version}` : ""}`
                  : "—"}
              </td>
            </tr>
          ))}
          {visibleSessions.length === 0 && !loading && (
            <tr>
              <td colSpan={6} className="admin-empty">
                No sessions found.
              </td>
            </tr>
          )}
        </tbody>
      </table>

      <div className="admin-pagination">
        {loading && <span>Loading…</span>}
        {!loading && hasMore && (
          <button type="button" className="admin-btn" onClick={loadMore}>
            Load more
          </button>
        )}
      </div>
    </div>
  );
}
