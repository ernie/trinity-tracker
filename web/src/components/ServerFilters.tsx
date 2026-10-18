import { useEffect, useMemo } from "react";
import type { ServerStatus } from "../types";
import { SourceFilter } from "./SourceFilter";
import { NavScroller } from "./NavScroller";
import { MODE_PROFILES } from "./ServerCard";
import { formatGameType } from "./MatchCard";

export interface ServerFilterState {
  source: string; // '' = all
  gameType: string; // '' = all (normalized lowercase)
  mode: string; // '' = all (g_mode value)
}

export const EMPTY_FILTERS: ServerFilterState = {
  source: "",
  gameType: "",
  mode: "",
};

const STORAGE_KEY = "q3a_server_filters";

export function loadServerFilters(): ServerFilterState {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return EMPTY_FILTERS;
    const parsed = JSON.parse(raw) as Partial<ServerFilterState>;
    return {
      source: typeof parsed.source === "string" ? parsed.source : "",
      gameType: typeof parsed.gameType === "string" ? parsed.gameType : "",
      mode: typeof parsed.mode === "string" ? parsed.mode : "",
    };
  } catch {
    return EMPTY_FILTERS;
  }
}

export function applyServerFilters(
  servers: ServerStatus[],
  f: ServerFilterState,
): ServerStatus[] {
  return servers.filter((s) => {
    if (f.source && s.source !== f.source) return false;
    if (f.gameType && (s.game_type || "").toLowerCase() !== f.gameType)
      return false;
    if (f.mode && (s.server_vars?.g_mode ?? "0") !== f.mode) return false;
    return true;
  });
}

interface ServerFiltersProps {
  servers: ServerStatus[];
  filters: ServerFilterState;
  onChange: (next: ServerFilterState) => void;
}

// Each filter row shows only when the live cards carry two or more values
// for it.
export function ServerFilters({
  servers,
  filters,
  onChange,
}: ServerFiltersProps) {
  const { gameTypes, modes } = useMemo(() => {
    const gt = new Set<string>();
    const md = new Set<string>();
    for (const s of servers) {
      if (s.game_type) gt.add(s.game_type.toLowerCase());
      md.add(s.server_vars?.g_mode ?? "0");
    }
    return {
      gameTypes: Array.from(gt).sort(),
      modes: Array.from(md).sort(),
    };
  }, [servers]);

  // In an effect so an initial render with stored values writes once.
  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(filters));
  }, [filters]);

  // A stored value missing from the live list would leave an empty grid
  // with no explanation. SourceFilter checks the source against
  // /api/sources itself.
  useEffect(() => {
    const next: Partial<ServerFilterState> = {};
    if (
      filters.gameType &&
      servers.length > 0 &&
      !gameTypes.includes(filters.gameType)
    ) {
      next.gameType = "";
    }
    if (filters.mode && servers.length > 0 && !modes.includes(filters.mode)) {
      next.mode = "";
    }
    if (Object.keys(next).length > 0) {
      onChange({ ...filters, ...next });
    }
  }, [servers, filters, gameTypes, modes, onChange]);

  const showGameType = gameTypes.length > 1;
  const showMode = modes.length > 1;

  const hasActive = !!filters.source || !!filters.gameType || !!filters.mode;

  // SourceFilter renders nothing for a single source, so with no other
  // rows there's nothing to show.
  if (!showGameType && !showMode && !hasActive) {
    return null;
  }

  return (
    <div className="server-filters">
      <SourceFilter
        value={filters.source}
        onChange={(v) => onChange({ ...filters, source: v })}
      />
      {showGameType && (
        <div
          className="server-filter-group"
          role="radiogroup"
          aria-label="Filter by game type"
        >
          <NavScroller scrollClassName="filter-chips__scroll">
            <div className="filter-chips__strip">
              <button
                type="button"
                className={`game-type-btn ${filters.gameType === "" ? "active" : ""}`}
                onClick={() => onChange({ ...filters, gameType: "" })}
              >
                All
              </button>
              {gameTypes.map((gt) => (
                <button
                  type="button"
                  key={gt}
                  className={`game-type-btn ${filters.gameType === gt ? "active" : ""}`}
                  onClick={() => onChange({ ...filters, gameType: gt })}
                >
                  {formatGameType(gt)}
                </button>
              ))}
            </div>
          </NavScroller>
        </div>
      )}
      {showMode && (
        <ModeFilterGroup
          label="Mode"
          shortLabel="Mode:"
          modes={MODE_PROFILES}
          available={modes}
          value={filters.mode}
          onChange={(v) => onChange({ ...filters, mode: v })}
        />
      )}
      {hasActive && (
        <button
          type="button"
          className="clear-filters-btn"
          onClick={() => onChange(EMPTY_FILTERS)}
        >
          Clear
        </button>
      )}
    </div>
  );
}

export interface ModeFilterGroupProps {
  label: string;
  shortLabel: string;
  modes: Record<string, { icon: string; label: string }>;
  available: string[];
  value: string;
  onChange: (next: string) => void;
}

export function ModeFilterGroup({
  label,
  shortLabel,
  modes,
  available,
  value,
  onChange,
}: ModeFilterGroupProps) {
  return (
    <div
      className="server-filter-group"
      role="radiogroup"
      aria-label={`Filter by ${label.toLowerCase()}`}
    >
      <span className="server-filter-label" aria-hidden="true">
        {shortLabel}
      </span>
      <NavScroller scrollClassName="filter-chips__scroll">
        <div className="filter-chips__strip">
          <button
            type="button"
            className={`mode-filter-btn ${value === "" ? "active" : ""}`}
            onClick={() => onChange("")}
            title={`All ${label}`}
          >
            All
          </button>
          {available.map((id) => {
            const mode = modes[id];
            if (!mode) return null;
            return (
              <button
                type="button"
                key={id}
                className={`mode-filter-btn icon-btn ${value === id ? "active" : ""}`}
                onClick={() => onChange(id)}
                title={`${label}: ${mode.label}`}
                aria-label={`${label}: ${mode.label}`}
              >
                <img src={mode.icon} alt="" />
              </button>
            );
          })}
        </div>
      </NavScroller>
    </div>
  );
}
