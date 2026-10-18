import { useState, useEffect, useCallback, useMemo } from "react";
import { apiFetch } from "../../authFetch";
import { formatDateTime } from "../../utils/formatters";
import {
  AdminAuditFilters,
  EMPTY_ADMIN_AUDIT_FILTERS,
  loadAdminAuditFilters,
  resolveAuditSince,
  SYSTEM_ACTOR_SENTINEL,
  type AdminAuditFilterState,
  adminAuditFiltersActive,
} from "./AdminAuditFilters";

interface AuditEntry {
  id: number;
  source: string;
  actor_user_id?: number;
  actor_username?: string;
  action: string;
  detail?: string;
  created_at: string;
}

const DEFAULT_LIMIT = 100;

export function AdminAudit() {
  const [rows, setRows] = useState<AuditEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [filters, setFilters] = useState<AdminAuditFilterState>(
    loadAdminAuditFilters,
  );

  // Actor filters on the server so it covers every row past the fetched
  // page. ?actor= has no way to say "no actor", so the system sentinel
  // filters here with Source and Action.
  const url = useMemo(() => {
    const params = new URLSearchParams();
    params.set("limit", String(DEFAULT_LIMIT));
    const since = resolveAuditSince(filters);
    if (since) params.set("since", since);
    if (filters.actor !== null && filters.actor !== SYSTEM_ACTOR_SENTINEL) {
      params.set("actor", filters.actor);
    }
    return `/api/admin/audit?${params.toString()}`;
  }, [filters]);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const res = await apiFetch(url);
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(
          data.error || `Couldn't load the audit log (${res.status}).`,
        );
      }
      const data: AuditEntry[] = (await res.json()) || [];
      setRows(data);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Couldn't load the audit log.",
      );
      setRows([]);
    } finally {
      setLoading(false);
    }
  }, [url]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load();
  }, [load]);

  const knownSources = useMemo(
    () => Array.from(new Set(rows.map((r) => r.source))).sort(),
    [rows],
  );
  const knownActors = useMemo(() => {
    const set = new Set<string>();
    for (const r of rows) set.add(r.actor_username || SYSTEM_ACTOR_SENTINEL);
    return Array.from(set).sort();
  }, [rows]);
  const knownActions = useMemo(
    () => Array.from(new Set(rows.map((r) => r.action))).sort(),
    [rows],
  );

  const visibleRows = useMemo(() => {
    return rows.filter((r) => {
      if (filters.hiddenSources.includes(r.source)) return false;
      if (filters.actor === SYSTEM_ACTOR_SENTINEL && r.actor_username)
        return false;
      if (filters.hiddenActions.includes(r.action)) return false;
      // The API takes only `since`, so a custom range's end applies here.
      if (filters.datePreset === "custom" && filters.customUntil) {
        const until = new Date(filters.customUntil);
        if (!isNaN(until.getTime()) && new Date(r.created_at) > until)
          return false;
      }
      return true;
    });
  }, [rows, filters]);

  const hasActive = adminAuditFiltersActive(filters);

  return (
    <div className="admin-audit">
      <div className="admin-section-header">
        <h2>
          Audit log
          <span className="admin-section-header__count">
            {" "}
            ({visibleRows.length})
          </span>
        </h2>
        <div style={{ display: "flex", gap: "var(--space-2)" }}>
          {hasActive && (
            <button
              type="button"
              className="admin-btn"
              onClick={() => setFilters(EMPTY_ADMIN_AUDIT_FILTERS)}
            >
              Clear filters
            </button>
          )}
          <button
            type="button"
            className="admin-btn"
            onClick={load}
            disabled={loading}
          >
            {loading ? "Loading…" : "Refresh"}
          </button>
        </div>
      </div>

      <AdminAuditFilters
        state={filters}
        onChange={setFilters}
        knownSources={knownSources}
        knownActors={knownActors}
        knownActions={knownActions}
      />

      {error && <div className="admin-audit-error">{error}</div>}

      <table className="admin-data-table admin-audit-table">
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
          {visibleRows.length === 0 && !loading && (
            <tr>
              <td colSpan={5} className="admin-audit-empty">
                No audit entries found.
              </td>
            </tr>
          )}
          {visibleRows.map((r) => (
            <tr key={r.id}>
              <td data-label="When">{formatDateTime(r.created_at)}</td>
              <td data-label="Source">{r.source}</td>
              <td data-label="Actor">
                {r.actor_username || (
                  <span className="admin-audit-system">system</span>
                )}
              </td>
              <td data-label="Action">
                <span
                  className={`admin-audit-action admin-audit-action-${r.action}`}
                >
                  {r.action}
                </span>
              </td>
              <td data-label="Detail">{r.detail || ""}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
