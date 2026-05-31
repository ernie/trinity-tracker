import { useEffect, useRef, useState } from "react";
import { type DatePreset } from "./AdminSessionsFilters";

// Source/Action use the hidden-set model (small bounded enums); Actor
// scales unbounded as the user base grows, so it's a single-select
// typeahead instead — null = no filter, sentinel = system rows only,
// any other string = exact actor_username (pushed to backend).
export interface AdminAuditFilterState {
  hiddenSources: string[];
  actor: string | null;
  hiddenActions: string[];
  datePreset: DatePreset;
  customSince: string;
  customUntil: string;
}

export const SYSTEM_ACTOR_SENTINEL = "__system__";

export const EMPTY_ADMIN_AUDIT_FILTERS: AdminAuditFilterState = {
  hiddenSources: [],
  actor: null,
  hiddenActions: [],
  datePreset: "",
  customSince: "",
  customUntil: "",
};

const STORAGE_KEY = "q3a_admin_audit_filters";

export function loadAdminAuditFilters(): AdminAuditFilterState {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return EMPTY_ADMIN_AUDIT_FILTERS;
    const parsed = JSON.parse(raw) as Partial<AdminAuditFilterState>;
    const preset = parsed.datePreset;
    return {
      hiddenSources: Array.isArray(parsed.hiddenSources)
        ? parsed.hiddenSources.filter((x): x is string => typeof x === "string")
        : [],
      actor: typeof parsed.actor === "string" ? parsed.actor : null,
      hiddenActions: Array.isArray(parsed.hiddenActions)
        ? parsed.hiddenActions.filter((x): x is string => typeof x === "string")
        : [],
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
    return EMPTY_ADMIN_AUDIT_FILTERS;
  }
}

// Resolve the active filter window to the RFC3339 string the backend
// audit endpoint accepts. Mirrors the date-range logic in
// AdminSessionsFilters; the audit endpoint only takes `since` (no
// `until`) so we surface that limitation by ignoring customUntil here.
export function resolveAuditSince(
  state: AdminAuditFilterState,
): string | undefined {
  if (state.datePreset === "custom") {
    if (!state.customSince) return undefined;
    const t = new Date(state.customSince);
    if (isNaN(t.getTime())) return undefined;
    return t.toISOString();
  }
  const ms: Record<DatePreset, number> = {
    "": 0,
    "24h": 24 * 60 * 60 * 1000,
    "7d": 7 * 24 * 60 * 60 * 1000,
    "30d": 30 * 24 * 60 * 60 * 1000,
    custom: 0,
  };
  const span = ms[state.datePreset];
  if (!span) return undefined;
  return new Date(Date.now() - span).toISOString();
}

interface Props {
  state: AdminAuditFilterState;
  onChange: (next: AdminAuditFilterState) => void;
  // Categorical universes derived from the rendered rows. Pruning is
  // best-effort: we don't try to keep a long-tail history of every
  // actor that ever existed, so dropping a stale hidden value once it
  // hasn't been seen across a full fetch keeps the panel honest.
  knownSources: string[];
  knownActors: string[];
  knownActions: string[];
}

export function AdminAuditFilters({
  state,
  onChange,
  knownSources,
  knownActors,
  knownActions,
}: Props) {
  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
  }, [state]);

  // Stale-prune hidden categorical values once data has populated.
  const sourcesValidated = useRef(false);
  const actionsValidated = useRef(false);
  useEffect(() => {
    let next = state;
    if (!sourcesValidated.current && knownSources.length > 0) {
      const valid = new Set(knownSources);
      const pruned = state.hiddenSources.filter((s) => valid.has(s));
      if (pruned.length !== state.hiddenSources.length)
        next = { ...next, hiddenSources: pruned };
      sourcesValidated.current = true;
    }
    if (!actionsValidated.current && knownActions.length > 0) {
      const valid = new Set(knownActions);
      const pruned = state.hiddenActions.filter((a) => valid.has(a));
      if (pruned.length !== state.hiddenActions.length)
        next = { ...next, hiddenActions: pruned };
      actionsValidated.current = true;
    }
    if (next !== state) onChange(next);
  }, [state, knownSources, knownActions, onChange]);

  return (
    <div className="admin-filters-panel">
      <CategoricalSection
        title="Source"
        values={knownSources}
        hidden={state.hiddenSources}
        onChange={(v) => onChange({ ...state, hiddenSources: v })}
      />
      <CategoricalSection
        title="Action"
        values={knownActions}
        hidden={state.hiddenActions}
        onChange={(v) => onChange({ ...state, hiddenActions: v })}
        renderLabel={(action) => (
          <span className={`admin-audit-action admin-audit-action-${action}`}>
            {action}
          </span>
        )}
      />
      <div className="static-section static-section--row">
        <ActorSection
          knownActors={knownActors}
          actor={state.actor}
          onChange={(v) => onChange({ ...state, actor: v })}
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

// ── Actor typeahead ──────────────────────────────────────────────────
// Single-select. Real actor names get pushed to the backend via
// ?actor=<name> for accurate filtering across all rows; the system
// pseudo-actor is filtered client-side because the backend's exact
// match against actor_username can't represent NULL.

interface ActorSectionProps {
  knownActors: string[];
  actor: string | null;
  onChange: (v: string | null) => void;
}

function ActorSection({ knownActors, actor, onChange }: ActorSectionProps) {
  const [query, setQuery] = useState("");

  // Only suggest matches once the user has started typing — an unfocused,
  // empty input shouldn't render a name list.
  const matches =
    query.length === 0
      ? []
      : knownActors
          .filter((a) => a.toLowerCase().includes(query.toLowerCase()))
          .slice(0, 8);

  const display = (a: string) => (a === SYSTEM_ACTOR_SENTINEL ? "system" : a);

  if (actor !== null) {
    return (
      <StaticSection
        title="Actor"
        action={{
          label: "Clear",
          onClick: () => {
            onChange(null);
            setQuery("");
          },
        }}
      >
        <div className="selected-player">
          <span>{display(actor)}</span>
          <button
            type="button"
            onClick={() => {
              onChange(null);
              setQuery("");
            }}
          >
            Clear
          </button>
        </div>
      </StaticSection>
    );
  }

  return (
    <StaticSection title="Actor">
      <div className="player-typeahead">
        <input
          type="text"
          placeholder="Type an actor name…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        {matches.length > 0 && (
          <ul className="player-results">
            {matches.map((a) => (
              <li
                key={a}
                onClick={() => {
                  onChange(a);
                  setQuery("");
                }}
              >
                {display(a)}
              </li>
            ))}
          </ul>
        )}
      </div>
    </StaticSection>
  );
}

// ── Reusable categorical (multi-switch) section ──────────────────────

interface CategoricalSectionProps {
  title: string;
  values: string[];
  hidden: string[];
  onChange: (v: string[]) => void;
  renderLabel?: (value: string) => React.ReactNode;
}

function CategoricalSection({
  title,
  values,
  hidden,
  onChange,
  renderLabel,
}: CategoricalSectionProps) {
  if (values.length === 0) return null;
  return (
    <FilterSection
      title={title}
      onAll={() => onChange([])}
      onNone={() => onChange([...values])}
    >
      {/* Two-column .filter-subgroup__body — Action labels render as
          .admin-audit-action chips that exceed the 80px-min --flat
          variant; sources/actors are usually short but inconsistency
          would hurt more than the wider columns. */}
      <div className="filter-subgroup__body">
        {values.map((v) => (
          <Switch
            key={v}
            checked={!hidden.includes(v)}
            onChange={() =>
              onChange(
                hidden.includes(v)
                  ? hidden.filter((x) => x !== v)
                  : [...hidden, v],
              )
            }
            label={renderLabel ? renderLabel(v) : v}
          />
        ))}
      </div>
    </FilterSection>
  );
}

// ── Date range (static, side-by-side with Actor) ──────────────────────

interface DateRangeSectionProps {
  preset: DatePreset;
  customSince: string;
  customUntil: string;
  onChange: (patch: Partial<AdminAuditFilterState>) => void;
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
      title="Since"
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
            <span>To (display only)</span>
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

// ── Reusable section shells ──────────────────────────────────────────

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

interface SwitchProps {
  checked: boolean;
  onChange: () => void;
  label: React.ReactNode;
}

function Switch({ checked, onChange, label }: SwitchProps) {
  return (
    <label className="switch" onClick={(e) => e.stopPropagation()}>
      <input type="checkbox" checked={checked} onChange={onChange} />
      <span className="switch__track" aria-hidden="true">
        <span className="switch__thumb" />
      </span>
      <span className="switch__label">{label}</span>
    </label>
  );
}

export function adminAuditFiltersActive(s: AdminAuditFilterState): boolean {
  return (
    s.hiddenSources.length > 0 ||
    s.actor !== null ||
    s.hiddenActions.length > 0 ||
    s.datePreset !== "" ||
    s.customSince !== "" ||
    s.customUntil !== ""
  );
}
