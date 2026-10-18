import { useEffect, useMemo, useRef, useState } from "react";
import type { Server, PlayerProfile } from "../../types";
import { useDebouncedValue } from "../../hooks/useDebouncedValue";

// As in ActivityFilters, `hiddenServers` names what's filtered out. Player
// and date are single-select because the API takes one of each.
export type DatePreset = "" | "24h" | "7d" | "30d" | "custom";

export interface AdminSessionsFilterState {
  hiddenServers: number[];
  playerId: number | null;
  playerLabel: string;
  datePreset: DatePreset;
  // datetime-local strings, set only for the custom preset.
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

// Custom times are read in the browser's time zone and sent as UTC.
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
}

export function AdminSessionsFilters({ servers, state, onChange }: Props) {
  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
  }, [state]);

  // A saved server that no longer exists could hide every row, so stale
  // ids are pruned once the server list arrives.
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
      <div className="static-section static-section--row">
        <PlayerSection
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
// Every session belongs to a server, so a source switch takes its state
// from its servers' switches.

function ServersSection({
  servers,
  hidden,
  onChange,
}: {
  servers: Server[];
  hidden: number[];
  onChange: (v: number[]) => void;
}) {
  // Sorted so the panel holds still as the server list updates.
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
            const next = new Set(hidden);
            sourceServers.forEach((s) => next.add(s.id));
            onChange(Array.from(next));
          } else {
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
              {/* preventDefault leaves collapsing to the caret, so a
                  click on the title toggles the switch. */}
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
  playerId: number | null;
  playerLabel: string;
  onPick: (player: PlayerProfile | null) => void;
}

function PlayerSection({ playerId, playerLabel, onPick }: PlayerSectionProps) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<PlayerProfile[]>([]);
  const debounced = useDebouncedValue(query, 200);

  useEffect(() => {
    if (debounced.length < 2) return;
    const ctrl = new AbortController();
    fetch(`/api/players?search=${encodeURIComponent(debounced)}&limit=10`, {
      signal: ctrl.signal,
    })
      .then((res) => (res.ok ? res.json() : []))
      .then((data: PlayerProfile[]) => setResults(data ?? []))
      .catch(() => {
        /* aborted */
      });
    return () => ctrl.abort();
  }, [debounced]);

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
              ? "Last 24 hours"
              : p === "7d"
                ? "Last 7 days"
                : p === "30d"
                  ? "Last 30 days"
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

// ── Collapsible section ──────────────────────────────────────────────

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

// ── Static section ───────────────────────────────────────────────────

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

// ── Toggle switch ────────────────────────────────────────────────────

interface SwitchProps {
  checked: boolean;
  // Only shows while unchecked, as the checkbox's native mixed state.
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

export function useStableServers(servers: Server[]): Server[] {
  return useMemo(() => servers.slice().sort((a, b) => a.id - b.id), [servers]);
}
