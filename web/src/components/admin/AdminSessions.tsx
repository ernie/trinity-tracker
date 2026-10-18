import { useState, useEffect, useCallback } from "react";
import { Link } from "react-router-dom";
import { apiFetch } from "../../authFetch";
import { ColoredText } from "../ColoredText";
import { SessionPlatform } from "../SessionPlatform";
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
  const [filters, setFilters] = useState<AdminSessionsFilterState>(
    loadAdminSessionsFilters,
  );

  const [sessions, setSessions] = useState<AdminSession[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [hasMore, setHasMore] = useState(false);

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
      // The API takes one server_id, so only a single visible server is
      // sent; several visible servers filter the returned page instead.
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
        throw new Error(data.error || "Couldn't load sessions.");
      }
      const data: AdminSession[] = (await res.json()) || [];
      setSessions(data);
      setHasMore(data.length === PAGE_SIZE);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Couldn't load sessions.");
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
      if (!res.ok) throw new Error("Couldn't load more sessions.");
      const data: AdminSession[] = (await res.json()) || [];
      setSessions((prev) => [...prev, ...data]);
      setHasMore(data.length === PAGE_SIZE);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Couldn't load more sessions.",
      );
    } finally {
      setLoading(false);
    }
  }, [buildUrl, sessions, loading]);

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
            <th>Platform</th>
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
              <td data-label="Platform">
                <SessionPlatform platform={s.platform} isVR={s.is_vr} />
              </td>
            </tr>
          ))}
          {visibleSessions.length === 0 && !loading && (
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
        {!loading && hasMore && (
          <button type="button" className="admin-btn" onClick={loadMore}>
            Load more
          </button>
        )}
      </div>
    </div>
  );
}
