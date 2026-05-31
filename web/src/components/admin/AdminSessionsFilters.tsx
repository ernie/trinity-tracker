import { useEffect, useMemo, useRef, useState } from "react";
import type { Server, PlayerProfile } from "../../types";
import { useDebouncedValue } from "../../hooks/useDebouncedValue";

// "Hidden" model mirrors ActivityFilters: items present in `hiddenServers`
// are filtered out of the listing. Empty set = all visible. The player and
// date filters are single-select rather than multi-select because the
// backend only takes one of each — they remain in the same vocabulary as
// the rest of the structured-filter recipe but with a typeahead and
// preset chips inside their <details> bodies.
export type DatePreset = "" | "24h" | "7d" | "30d" | "custom";

export interface AdminSessionsFilterState {
  hiddenServers: number[];
  playerId: number | null;
  playerLabel: string;
  datePreset: DatePreset;
  // ISO-8601 strings only when datePreset === 'custom'. Stored as strings
  // (rather than Date) so localStorage round-trips losslessly.
  customSince: string;
  customUntil: string;
}

export const EMPTY_ADMIN_SESSIONS_FILTERS: AdminSessionsFilterState = {
  hiddenServers: [],
  playerId: null,
  playerLabel: "",
  datePreset: "",
  customSince: "",
  customUntil: "",
};

const STORAGE_KEY = "q3a_admin_sessions_filters";

export function loadAdminSessionsFilters(): AdminSessionsFilterState {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return EMPTY_ADMIN_SESSIONS_FILTERS;
    const parsed = JSON.parse(raw) as Partial<AdminSessionsFilterState>;
    const preset = parsed.datePreset;
    return {
      hiddenServers: Array.isArray(parsed.hiddenServers)
        ? parsed.hiddenServers.filter((x): x is number => typeof x === "number")
        : [],
      playerId: typeof parsed.playerId === "number" ? parsed.playerId : null,
      playerLabel:
        typeof parsed.playerLabel === "string" ? parsed.playerLabel : "",
      datePreset:
        preset === "24h" ||
        preset === "7d" ||
        preset === "30d" ||
        preset === "custom"
          ? preset
          : "",
      customSince:
        typeof parsed.customSince === "string" ? parsed.customSince : "",
      customUntil:
        typeof parsed.customUntil === "string" ? parsed.customUntil : "",
    };
  } catch {
    return EMPTY_ADMIN_SESSIONS_FILTERS;
  }
}

// Resolve the active filter window to the RFC3339 strings the backend
// accepts. Presets compute a relative window from "now"; custom uses
// the literal datetime-local strings (interpreted in the browser's
// local zone, then serialized to UTC).
export function resolveDateRange(state: AdminSessionsFilterState): {
  since?: string;
  until?: string;
} {
  if (state.datePreset === "custom") {
    const out: { since?: string; until?: string } = {};
    if (state.customSince) {
      const t = new Date(state.customSince);
      if (!isNaN(t.getTime())) out.since = t.toISOString();
    }
    if (state.customUntil) {
      const t = new Date(state.customUntil);
      if (!isNaN(t.getTime())) out.until = t.toISOString();
    }
    return out;
  }
  const now = new Date();
  const ms: Record<DatePreset, number> = {
    "": 0,
    "24h": 24 * 60 * 60 * 1000,
    "7d": 7 * 24 * 60 * 60 * 1000,
    "30d": 30 * 24 * 60 * 60 * 1000,
    custom: 0,
  };
  const span = ms[state.datePreset];
  if (!span) return {};
  return { since: new Date(now.getTime() - span).toISOString() };
}

interface Props {
  servers: Server[];
  state: AdminSessionsFilterState;
  onChange: (next: AdminSessionsFilterState) => void;
  // Player typeahead — fetches lazily via the parent's authenticated session.
  token: string;
}

export function AdminSessionsFilters({
  servers,
  state,
  onChange,
  token,
}: Props) {
  // Persist on every change. Doing it in an effect (rather than the
  // setter) avoids a double-write on the first render that hydrates
  // from localStorage.
  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
  }, [state]);

  // Stale-prune hiddenServers once the live server list arrives. Same
  // pattern as ActivityFilters/ServerFilters — protects users from
  // ending up with a localStorage value that filters every row out.
  const serversValidated = useRef(false);
  useEffect(() => {
    if (!serversValidated.current && servers.length > 0) {
      const valid = new Set(servers.map((s) => s.id));
      const pruned = state.hiddenServers.filter((id) => valid.has(id));
      if (pruned.length !== state.hiddenServers.length) {
        onChange({ ...state, hiddenServers: pruned });
      }
      serversValidated.current = true;
    }
  }, [servers, state, onChange]);

  return (
    <div className="admin-filters-panel">
      <ServersSection
        servers={servers}
        hidden={state.hiddenServers}
        onChange={(v) => onChange({ ...state, hiddenServers: v })}
      />
      {/* Player + Date range share a horizontal row below the
          collapsible Sources & servers section. Neither has a meaningful
          collapsed state (Player is always either an input or a chip;
          Date range is always preset chips), so no caret. The row sits
          inside the same admin-filters-panel border with a shared
          top-rule provided by .static-section--row. */}
      <div className="static-section static-section--row">
        <PlayerSection
          token={token}
          playerId={state.playerId}
          playerLabel={state.playerLabel}
          onPick={(p) =>
            onChange({
              ...state,
              playerId: p?.id ?? null,
              playerLabel: p?.clean_name ?? "",
            })
          }
        />
        <DateRangeSection
          preset={state.datePreset}
          customSince={state.customSince}
          customUntil={state.customUntil}
          onChange={(patch) => onChange({ ...state, ...patch })}
        />
      </div>
    </div>
  );
}

// ── Sources & servers ────────────────────────────────────────────────
// Mirrors the activity drawer's recipe: each source is a
// <details className="filter-subgroup"> with the source switch inside
// the summary; servers hang as switches in the body. Source switch
// derives its state from the children — visible if any child visible,
// mixed if some-but-not-all hidden, off if all hidden — so we don't
// need a separate hiddenSources field on the state (every session is
// server-bound, unlike activity which also has source-only events).

function ServersSection({
  servers,
  hidden,
  onChange,
}: {
  servers: Server[];
  hidden: number[];
  onChange: (v: number[]) => void;
}) {
  // Group servers by source. Sort sources alphabetically and servers
  // within each source by id so panel order stays stable across the
  // websocket-driven server list updates.
  const bySource = useMemo(() => {
    const map = new Map<string, Server[]>();
    for (const s of servers) {
      const arr = map.get(s.source) ?? [];
      arr.push(s);
      map.set(s.source, arr);
    }
    for (const arr of map.values()) arr.sort((a, b) => a.id - b.id);
    return Array.from(map.entries()).sort((a, b) => a[0].localeCompare(b[0]));
  }, [servers]);

  if (servers.length === 0) return null;

  const allIds = servers.map((s) => s.id);

  return (
    <FilterSection
      title="Sources & servers"
      onAll={() => onChange([])}
      onNone={() => onChange(allIds)}
    >
      {bySource.map(([source, sourceServers]) => {
        const hiddenCount = sourceServers.filter((s) =>
          hidden.includes(s.id),
        ).length;
        const sourceVisible = hiddenCount < sourceServers.length;
        const indeterminate =
          hiddenCount > 0 && hiddenCount < sourceServers.length;

        const toggleSource = () => {
          if (sourceVisible) {
            // Hide all children: union their ids into the hidden set.
            const next = new Set(hidden);
            sourceServers.forEach((s) => next.add(s.id));
            onChange(Array.from(next));
          } else {
            // Show all children: drop their ids from the hidden set.
            const ids = new Set(sourceServers.map((s) => s.id));
            onChange(hidden.filter((id) => !ids.has(id)));
          }
        };

        return (
          <details key={source} className="filter-subgroup" open>
            <summary className="filter-subgroup__header">
              <span className="filter-subgroup__caret" aria-hidden="true">
                ▸
              </span>
              {/* Click on the title-wrap toggles the source switch
                  (preventDefault keeps <details> open/close on the caret
                  alone). Click on the caret-only area still toggles the
                  details collapse. */}
              <span
                className="filter-subgroup__title-wrap"
                onClick={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  toggleSource();
                }}
              >
                <Switch
                  checked={sourceVisible}
                  indeterminate={indeterminate}
                  onChange={toggleSource}
                  label={source}
                />
              </span>
            </summary>
            <div className="filter-subgroup__body">
              {sourceServers.map((s) => (
                <Switch
                  key={s.id}
                  checked={!hidden.includes(s.id)}
                  onChange={() =>
                    onChange(
                      hidden.includes(s.id)
                        ? hidden.filter((id) => id !== s.id)
                        : [...hidden, s.id],
                    )
                  }
                  label={s.key}
                />
              ))}
            </div>
          </details>
        );
      })}
    </FilterSection>
  );
}

// ── Player typeahead ─────────────────────────────────────────────────

interface PlayerSectionProps {
  token: string;
  playerId: number | null;
  playerLabel: string;
  onPick: (player: PlayerProfile | null) => void;
}

function PlayerSection({
  token,
  playerId,
  playerLabel,
  onPick,
}: PlayerSectionProps) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<PlayerProfile[]>([]);
  const debounced = useDebouncedValue(query, 200);

  useEffect(() => {
    if (debounced.length < 2) return;
    const ctrl = new AbortController();
    fetch(`/api/players?search=${encodeURIComponent(debounced)}&limit=10`, {
      headers: { Authorization: `Bearer ${token}` },
      signal: ctrl.signal,
    })
      .then((res) => (res.ok ? res.json() : []))
      .then((data: PlayerProfile[]) => setResults(data ?? []))
      .catch(() => {
        /* aborted */
      });
    return () => ctrl.abort();
  }, [debounced, token]);

  // Hide stored results once the query is too short.
  const displayResults = debounced.length >= 2 ? results : [];

  return (
    <StaticSection
      title="Player"
      action={
        playerId !== null
          ? {
              label: "Clear",
              onClick: () => {
                onPick(null);
                setQuery("");
              },
            }
          : undefined
      }
    >
      {playerId !== null ? (
        <div className="selected-player">
          <span>{playerLabel || `Player #${playerId}`}</span>
          <button
            type="button"
            onClick={() => {
              onPick(null);
              setQuery("");
            }}
          >
            Clear
          </button>
        </div>
      ) : (
        <div className="player-typeahead">
          <input
            type="text"
            placeholder="Search players…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          {displayResults.length > 0 && (
            <ul className="player-results">
              {displayResults.map((p) => (
                <li
                  key={p.id}
                  onClick={() => {
                    onPick(p);
                    setQuery("");
                    setResults([]);
                  }}
                >
                  {p.clean_name}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </StaticSection>
  );
}

// ── Date range ───────────────────────────────────────────────────────

interface DateRangeSectionProps {
  preset: DatePreset;
  customSince: string;
  customUntil: string;
  onChange: (patch: Partial<AdminSessionsFilterState>) => void;
}

function DateRangeSection({
  preset,
  customSince,
  customUntil,
  onChange,
}: DateRangeSectionProps) {
  const hasValue = preset !== "" || customSince !== "" || customUntil !== "";
  return (
    <StaticSection
      title="Date range"
      action={
        hasValue
          ? {
              label: "Clear",
              onClick: () =>
                onChange({ datePreset: "", customSince: "", customUntil: "" }),
            }
          : undefined
      }
    >
      <div className="admin-preset-chips">
        {(["24h", "7d", "30d", "custom"] as const).map((p) => (
          <button
            key={p}
            type="button"
            className={`admin-preset-chip ${preset === p ? "active" : ""}`}
            onClick={() => onChange({ datePreset: preset === p ? "" : p })}
          >
            {p === "24h"
              ? "Last 24h"
              : p === "7d"
                ? "Last 7d"
                : p === "30d"
                  ? "Last 30d"
                  : "Custom"}
          </button>
        ))}
      </div>
      {preset === "custom" && (
        <div className="admin-custom-date-range">
          <label className="admin-custom-date-range__field">
            <span>From</span>
            <input
              type="datetime-local"
              value={customSince}
              onChange={(e) => onChange({ customSince: e.target.value })}
            />
          </label>
          <label className="admin-custom-date-range__field">
            <span>To</span>
            <input
              type="datetime-local"
              value={customUntil}
              onChange={(e) => onChange({ customUntil: e.target.value })}
            />
          </label>
        </div>
      )}
    </StaticSection>
  );
}

// ── Collapsible section (carets, used by Sources & servers) ─────────

interface FilterSectionProps {
  title: string;
  onAll: () => void;
  onNone?: () => void;
  children: React.ReactNode;
}

function FilterSection({ title, onAll, onNone, children }: FilterSectionProps) {
  return (
    <details className="filter-section" open>
      <summary className="filter-section__header">
        <span className="filter-section__caret" aria-hidden="true">
          ▸
        </span>
        <span className="filter-section__title">{title}</span>
        <span
          className="filter-section__actions"
          onClick={(e) => e.stopPropagation()}
        >
          <button
            type="button"
            onClick={(e) => {
              e.preventDefault();
              onAll();
            }}
          >
            All
          </button>
          {onNone && (
            <button
              type="button"
              onClick={(e) => {
                e.preventDefault();
                onNone();
              }}
            >
              None
            </button>
          )}
        </span>
      </summary>
      <div className="filter-section__body">{children}</div>
    </details>
  );
}

// ── Static (non-collapsible) section — for Player + Date range ──────
// No caret because the content is always shown — Player is either an
// input or a chip, Date range is always preset chips. Single optional
// action on the right (typically "Clear"). Visually shares the title
// vocabulary with .filter-section__title so the panel reads as one set.

interface StaticSectionProps {
  title: string;
  action?: { label: string; onClick: () => void };
  children: React.ReactNode;
}

function StaticSection({ title, action, children }: StaticSectionProps) {
  return (
    <div className="static-section">
      <div className="static-section__header">
        <span className="static-section__title">{title}</span>
        {action && (
          <span className="filter-section__actions">
            <button type="button" onClick={action.onClick}>
              {action.label}
            </button>
          </span>
        )}
      </div>
      <div className="static-section__body">{children}</div>
    </div>
  );
}

// ── Toggle switch (mirrors ActivityFilters' Switch component) ────────

interface SwitchProps {
  checked: boolean;
  // When true (and !checked), the underlying checkbox renders the
  // browser's native indeterminate state and we paint .switch--mixed
  // to match — the source-level toggle uses this for "some children
  // hidden, some visible." Mirrors ActivityFilters' Switch.
  indeterminate?: boolean;
  onChange: () => void;
  label: string;
}

function Switch({ checked, indeterminate, onChange, label }: SwitchProps) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = !!indeterminate && !checked;
  }, [indeterminate, checked]);
  const stateClass = indeterminate && !checked ? "switch--mixed" : "";
  return (
    <label
      className={`switch ${stateClass}`}
      onClick={(e) => e.stopPropagation()}
    >
      <input ref={ref} type="checkbox" checked={checked} onChange={onChange} />
      <span className="switch__track" aria-hidden="true">
        <span className="switch__thumb" />
      </span>
      <span className="switch__label">{label}</span>
    </label>
  );
}

// Convenience: any active filter? Used by the parent to decide whether
// to render a "Clear filters" affordance somewhere else in the page.
export function adminSessionsFiltersActive(
  s: AdminSessionsFilterState,
): boolean {
  return (
    s.hiddenServers.length > 0 ||
    s.playerId !== null ||
    s.datePreset !== "" ||
    s.customSince !== "" ||
    s.customUntil !== ""
  );
}

// Re-export memo helper so the parent can keep referential stability if needed.
export function useStableServers(servers: Server[]): Server[] {
  return useMemo(() => servers.slice().sort((a, b) => a.id - b.id), [servers]);
}
