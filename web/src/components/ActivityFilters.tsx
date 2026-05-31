import { useEffect, useMemo, useRef } from "react";
import type { ServerStatus } from "../types";
import {
  ACTIVITY_FILTER_CATEGORIES,
  ACTIVITY_FILTER_ALL_KEYS,
  ACTIVITY_FILTER_DEFAULT_ON,
  type ActivityFilterKey,
} from "../constants/activityTaxonomy";
import { useSources } from "../hooks/useSources";
import { serverDisplay } from "../utils";

// "Hidden" model: items present in each set are filtered out. Empty
// sets = everything visible. Events have non-empty defaults (most off)
// per the spam-control policy. includeBots is orthogonal — a player-
// attribute filter applied across event types.
export interface ActivityFilterState {
  hiddenSources: string[];
  hiddenServers: number[];
  hiddenEvents: ActivityFilterKey[];
  hiddenGameTypes: string[];
  includeBots: boolean;
}

const DEFAULT_HIDDEN_EVENTS: ActivityFilterKey[] =
  ACTIVITY_FILTER_ALL_KEYS.filter((k) => !ACTIVITY_FILTER_DEFAULT_ON.has(k));

export const EMPTY_ACTIVITY_FILTERS: ActivityFilterState = {
  hiddenSources: [],
  hiddenServers: [],
  hiddenEvents: DEFAULT_HIDDEN_EVENTS,
  hiddenGameTypes: [],
  includeBots: false,
};

const STORAGE_KEY = "q3a_activity_filters";

export function loadActivityFilters(): ActivityFilterState {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return EMPTY_ACTIVITY_FILTERS;
    const parsed = JSON.parse(raw) as Partial<ActivityFilterState>;
    return {
      hiddenSources: Array.isArray(parsed.hiddenSources)
        ? parsed.hiddenSources.filter((x) => typeof x === "string")
        : [],
      hiddenServers: Array.isArray(parsed.hiddenServers)
        ? parsed.hiddenServers.filter((x) => typeof x === "number")
        : [],
      hiddenEvents: Array.isArray(parsed.hiddenEvents)
        ? (parsed.hiddenEvents.filter((x) =>
            ACTIVITY_FILTER_ALL_KEYS.includes(x as ActivityFilterKey),
          ) as ActivityFilterKey[])
        : DEFAULT_HIDDEN_EVENTS,
      hiddenGameTypes: Array.isArray(parsed.hiddenGameTypes)
        ? parsed.hiddenGameTypes.filter((x) => typeof x === "string")
        : [],
      includeBots:
        typeof parsed.includeBots === "boolean" ? parsed.includeBots : false,
    };
  } catch {
    return EMPTY_ACTIVITY_FILTERS;
  }
}

interface Props {
  state: ActivityFilterState;
  onChange: (next: ActivityFilterState) => void;
  servers: Map<number, ServerStatus>;
}

export function ActivityFilters({ state, onChange, servers }: Props) {
  const { sources, hasMultiple: hasMultipleSources } = useSources();

  const liveSources = useMemo(() => sources.map((s) => s.source), [sources]);
  const serversBySource = useMemo(() => {
    const map = new Map<string, Array<{ id: number; name: string }>>();
    for (const [id, status] of servers.entries()) {
      if (!status.key || !status.source) continue;
      const arr = map.get(status.source) ?? [];
      arr.push({
        id,
        name: serverDisplay(status.source, status.key, { hasMultipleSources }),
      });
      map.set(status.source, arr);
    }
    for (const arr of map.values()) arr.sort((a, b) => a.id - b.id);
    return map;
  }, [servers, hasMultipleSources]);
  const liveGameTypes = useMemo(() => {
    const set = new Set<string>();
    for (const [, status] of servers.entries()) {
      if (status.game_type) set.add(status.game_type.toLowerCase());
    }
    return Array.from(set).sort();
  }, [servers]);

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
  }, [state]);

  // Stale pruning per ServerFilters.tsx pattern.
  const sourcesValidated = useRef(false);
  const gameTypesValidated = useRef(false);
  const serversValidated = useRef(false);
  useEffect(() => {
    let next = state;
    if (!sourcesValidated.current && sources.length > 0) {
      const valid = new Set(liveSources);
      const pruned = state.hiddenSources.filter((s) => valid.has(s));
      if (pruned.length !== state.hiddenSources.length)
        next = { ...next, hiddenSources: pruned };
      sourcesValidated.current = true;
    }
    if (!gameTypesValidated.current && liveGameTypes.length > 0) {
      const valid = new Set(liveGameTypes);
      const pruned = state.hiddenGameTypes.filter((g) => valid.has(g));
      if (pruned.length !== state.hiddenGameTypes.length)
        next = { ...next, hiddenGameTypes: pruned };
      gameTypesValidated.current = true;
    }
    if (!serversValidated.current && servers.size > 0) {
      const pruned = state.hiddenServers.filter((id) => servers.has(id));
      if (pruned.length !== state.hiddenServers.length)
        next = { ...next, hiddenServers: pruned };
      serversValidated.current = true;
    }
    if (next !== state) onChange(next);
  }, [state, liveSources, liveGameTypes, servers, sources, onChange]);

  const setHiddenSources = (v: string[]) =>
    onChange({ ...state, hiddenSources: v });
  const setHiddenServers = (v: number[]) =>
    onChange({ ...state, hiddenServers: v });
  const setHiddenEvents = (v: ActivityFilterKey[]) =>
    onChange({ ...state, hiddenEvents: v });
  const setHiddenGameTypes = (v: string[]) =>
    onChange({ ...state, hiddenGameTypes: v });

  return (
    <div className="activity-filters-panel">
      <div className="filter-toplevel">
        <Switch
          checked={state.includeBots}
          onChange={() =>
            onChange({ ...state, includeBots: !state.includeBots })
          }
          label="Include bot activity"
        />
      </div>
      <SourcesSection
        liveSources={liveSources}
        serversBySource={serversBySource}
        hiddenSources={state.hiddenSources}
        hiddenServers={state.hiddenServers}
        onSourcesChange={setHiddenSources}
        onServersChange={setHiddenServers}
      />
      <EventsSection hidden={state.hiddenEvents} onChange={setHiddenEvents} />
      <GameTypesSection
        liveGameTypes={liveGameTypes}
        hidden={state.hiddenGameTypes}
        onChange={setHiddenGameTypes}
      />
    </div>
  );
}

// ── Sources & Servers ────────────────────────────────────────────────

interface SourcesSectionProps {
  liveSources: string[];
  serversBySource: Map<string, Array<{ id: number; name: string }>>;
  hiddenSources: string[];
  hiddenServers: number[];
  onSourcesChange: (v: string[]) => void;
  onServersChange: (v: number[]) => void;
}

function SourcesSection({
  liveSources,
  serversBySource,
  hiddenSources,
  hiddenServers,
  onSourcesChange,
  onServersChange,
}: SourcesSectionProps) {
  if (liveSources.length === 0 && serversBySource.size === 0) return null;

  const allSection = () => {
    onSourcesChange([]);
    onServersChange([]);
  };
  const noneSection = () => {
    onSourcesChange([...liveSources]);
    const allIds: number[] = [];
    for (const arr of serversBySource.values())
      for (const s of arr) allIds.push(s.id);
    onServersChange(allIds);
  };

  return (
    <FilterSection
      title="Sources & servers"
      onAll={allSection}
      onNone={noneSection}
    >
      {liveSources.map((src) => {
        const list = serversBySource.get(src) ?? [];
        const hiddenCount = list.filter((s) =>
          hiddenServers.includes(s.id),
        ).length;
        const sourceHidden = hiddenSources.includes(src);
        const allChildHidden = list.length > 0 && hiddenCount === list.length;
        const indeterminate =
          !sourceHidden && hiddenCount > 0 && hiddenCount < list.length;
        const sourceVisible = !sourceHidden && !allChildHidden;

        const toggleSource = () => {
          if (sourceVisible) {
            onSourcesChange(
              hiddenSources.includes(src)
                ? hiddenSources
                : [...hiddenSources, src],
            );
            onServersChange([
              ...new Set([...hiddenServers, ...list.map((s) => s.id)]),
            ]);
          } else {
            onSourcesChange(hiddenSources.filter((s) => s !== src));
            onServersChange(
              hiddenServers.filter((id) => !list.some((s) => s.id === id)),
            );
          }
        };

        return (
          <details key={src} className="filter-subgroup" open>
            <summary className="filter-subgroup__header">
              <span className="filter-subgroup__caret" aria-hidden="true">
                ▸
              </span>
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
                  label={src}
                />
              </span>
            </summary>
            <div className="filter-subgroup__body">
              {list.map((s) => (
                <Switch
                  key={s.id}
                  checked={!sourceHidden && !hiddenServers.includes(s.id)}
                  disabled={sourceHidden}
                  onChange={() =>
                    onServersChange(
                      hiddenServers.includes(s.id)
                        ? hiddenServers.filter((id) => id !== s.id)
                        : [...hiddenServers, s.id],
                    )
                  }
                  label={s.name}
                />
              ))}
              {list.length === 0 && (
                <span className="filter-empty">no live servers</span>
              )}
            </div>
          </details>
        );
      })}
    </FilterSection>
  );
}

// ── Event Types ──────────────────────────────────────────────────────

function EventsSection({
  hidden,
  onChange,
}: {
  hidden: ActivityFilterKey[];
  onChange: (v: ActivityFilterKey[]) => void;
}) {
  return (
    <FilterSection
      title="Event types"
      onAll={() => onChange([])}
      onNone={() => onChange([...ACTIVITY_FILTER_ALL_KEYS])}
    >
      {ACTIVITY_FILTER_CATEGORIES.map((cat) => {
        const catKeys = cat.entries.map((e) => e.key);
        const hiddenInCat = catKeys.filter((k) => hidden.includes(k)).length;
        const allHidden = hiddenInCat === catKeys.length;
        const indeterminate = hiddenInCat > 0 && !allHidden;

        const toggleCategory = () => {
          if (allHidden) onChange(hidden.filter((k) => !catKeys.includes(k)));
          else onChange([...new Set([...hidden, ...catKeys])]);
        };

        return (
          <details key={cat.id} className="filter-subgroup" open>
            <summary className="filter-subgroup__header">
              <span className="filter-subgroup__caret" aria-hidden="true">
                ▸
              </span>
              <span
                className="filter-subgroup__title-wrap"
                onClick={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  toggleCategory();
                }}
              >
                <Switch
                  checked={!allHidden}
                  indeterminate={indeterminate}
                  onChange={toggleCategory}
                  label={cat.label}
                />
              </span>
            </summary>
            <div className="filter-subgroup__body">
              {cat.entries.map((e) => (
                <Switch
                  key={e.key}
                  checked={!hidden.includes(e.key)}
                  onChange={() =>
                    onChange(
                      hidden.includes(e.key)
                        ? hidden.filter((k) => k !== e.key)
                        : [...hidden, e.key],
                    )
                  }
                  label={e.label}
                />
              ))}
            </div>
          </details>
        );
      })}
    </FilterSection>
  );
}

// ── Game Modes ───────────────────────────────────────────────────────

function GameTypesSection({
  liveGameTypes,
  hidden,
  onChange,
}: {
  liveGameTypes: string[];
  hidden: string[];
  onChange: (v: string[]) => void;
}) {
  if (liveGameTypes.length === 0) return null;
  return (
    <FilterSection
      title="Game modes"
      onAll={() => onChange([])}
      onNone={() => onChange([...liveGameTypes])}
    >
      <div className="filter-subgroup__body filter-subgroup__body--flat">
        {liveGameTypes.map((gt) => (
          <Switch
            key={gt}
            checked={!hidden.includes(gt)}
            onChange={() =>
              onChange(
                hidden.includes(gt)
                  ? hidden.filter((g) => g !== gt)
                  : [...hidden, gt],
              )
            }
            label={gt.toUpperCase()}
          />
        ))}
      </div>
    </FilterSection>
  );
}

// ── Section shell ────────────────────────────────────────────────────

interface FilterSectionProps {
  title: string;
  onAll: () => void;
  onNone: () => void;
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
          <button
            type="button"
            onClick={(e) => {
              e.preventDefault();
              onNone();
            }}
          >
            None
          </button>
        </span>
      </summary>
      <div className="filter-section__body">{children}</div>
    </details>
  );
}

// ── Toggle switch ────────────────────────────────────────────────────

interface SwitchProps {
  checked: boolean;
  indeterminate?: boolean;
  disabled?: boolean;
  onChange: () => void;
  label: string;
}

function Switch({
  checked,
  indeterminate,
  disabled,
  onChange,
  label,
}: SwitchProps) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = !!indeterminate && !checked;
  }, [indeterminate, checked]);
  const stateClass = disabled
    ? "switch--disabled"
    : indeterminate && !checked
      ? "switch--mixed"
      : "";
  return (
    <label
      className={`switch ${stateClass}`}
      onClick={(e) => e.stopPropagation()}
    >
      <input
        ref={ref}
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={onChange}
      />
      <span className="switch__track" aria-hidden="true">
        <span className="switch__thumb" />
      </span>
      <span className="switch__label">{label}</span>
    </label>
  );
}
