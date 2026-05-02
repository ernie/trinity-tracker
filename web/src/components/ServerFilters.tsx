import { useEffect, useMemo } from 'react'
import type { ServerStatus } from '../types'
import { SourceFilter } from './SourceFilter'
import { MOVEMENT_MODES, GAMEPLAY_MODES } from './ServerCard'
import { formatGameType } from './MatchCard'

export interface ServerFilterState {
  source: string         // '' = all
  gameType: string       // '' = all (normalized lowercase)
  movement: string       // '' = all (g_movement value)
  gameplay: string       // '' = all (g_gameplay value)
}

export const EMPTY_FILTERS: ServerFilterState = {
  source: '',
  gameType: '',
  movement: '',
  gameplay: '',
}

const STORAGE_KEY = 'q3a_server_filters'

export function loadServerFilters(): ServerFilterState {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return EMPTY_FILTERS
    const parsed = JSON.parse(raw) as Partial<ServerFilterState>
    return {
      source: typeof parsed.source === 'string' ? parsed.source : '',
      gameType: typeof parsed.gameType === 'string' ? parsed.gameType : '',
      movement: typeof parsed.movement === 'string' ? parsed.movement : '',
      gameplay: typeof parsed.gameplay === 'string' ? parsed.gameplay : '',
    }
  } catch {
    return EMPTY_FILTERS
  }
}

export function applyServerFilters(servers: ServerStatus[], f: ServerFilterState): ServerStatus[] {
  return servers.filter((s) => {
    if (f.source && s.source !== f.source) return false
    if (f.gameType && (s.game_type || '').toLowerCase() !== f.gameType) return false
    if (f.movement && (s.server_vars?.g_movement ?? '0') !== f.movement) return false
    if (f.gameplay && (s.server_vars?.g_gameplay ?? '0') !== f.gameplay) return false
    return true
  })
}

interface ServerFiltersProps {
  servers: ServerStatus[]
  filters: ServerFilterState
  onChange: (next: ServerFilterState) => void
}

// ServerFilters renders a toolbar above the live server grid. Each
// filter axis auto-hides when fewer than two distinct values appear
// across the live cards — so a single-source / single-mode install
// doesn't render a row of useless controls. Persisted to localStorage
// via loadServerFilters; pruning of stale values happens here once the
// data arrives, mirroring ActivityLog's pattern.
export function ServerFilters({ servers, filters, onChange }: ServerFiltersProps) {
  // Derive the distinct values for each axis from the current server list.
  const { gameTypes, movements, gameplays } = useMemo(() => {
    const gt = new Set<string>()
    const mv = new Set<string>()
    const gp = new Set<string>()
    for (const s of servers) {
      if (s.game_type) gt.add(s.game_type.toLowerCase())
      mv.add(s.server_vars?.g_movement ?? '0')
      gp.add(s.server_vars?.g_gameplay ?? '0')
    }
    return {
      gameTypes: Array.from(gt).sort(),
      movements: Array.from(mv).sort(),
      gameplays: Array.from(gp).sort(),
    }
  }, [servers])

  // Persist on change. Done in an effect (rather than the setter) so
  // an initial render with stored values doesn't double-write.
  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(filters))
  }, [filters])

  // Drop a persisted filter value once we know the live list doesn't
  // contain it — otherwise the user sees an empty grid with no signal
  // as to why. Server-list axis (source) is validated by SourceFilter
  // itself via /api/sources.
  useEffect(() => {
    const next: Partial<ServerFilterState> = {}
    if (filters.gameType && servers.length > 0 && !gameTypes.includes(filters.gameType)) {
      next.gameType = ''
    }
    if (filters.movement && servers.length > 0 && !movements.includes(filters.movement)) {
      next.movement = ''
    }
    if (filters.gameplay && servers.length > 0 && !gameplays.includes(filters.gameplay)) {
      next.gameplay = ''
    }
    if (Object.keys(next).length > 0) {
      onChange({ ...filters, ...next })
    }
  }, [servers, filters, gameTypes, movements, gameplays, onChange])

  const showGameType = gameTypes.length > 1
  const showMovement = movements.length > 1
  const showGameplay = gameplays.length > 1

  const hasActive =
    !!filters.source || !!filters.gameType || !!filters.movement || !!filters.gameplay

  // Hide the whole toolbar when nothing is filterable (single source,
  // single mode, single game type — e.g., a fresh single-server install).
  if (!showGameType && !showMovement && !showGameplay && !hasActive) {
    // SourceFilter renders nothing on single-source installs; if there
    // are no other axes either, skip the whole toolbar.
    return null
  }

  return (
    <div className="server-filters">
      <SourceFilter value={filters.source} onChange={(v) => onChange({ ...filters, source: v })} />
      {showGameType && (
        <div className="server-filter-group" role="radiogroup" aria-label="Filter by game type">
          <button
            type="button"
            className={`game-type-btn ${filters.gameType === '' ? 'active' : ''}`}
            onClick={() => onChange({ ...filters, gameType: '' })}
          >
            All
          </button>
          {gameTypes.map((gt) => (
            <button
              type="button"
              key={gt}
              className={`game-type-btn ${filters.gameType === gt ? 'active' : ''}`}
              onClick={() => onChange({ ...filters, gameType: gt })}
            >
              {formatGameType(gt)}
            </button>
          ))}
        </div>
      )}
      {showMovement && (
        <ModeFilterGroup
          label="Movement"
          shortLabel="M"
          modes={MOVEMENT_MODES}
          available={movements}
          value={filters.movement}
          onChange={(v) => onChange({ ...filters, movement: v })}
        />
      )}
      {showGameplay && (
        <ModeFilterGroup
          label="Gameplay"
          shortLabel="G"
          modes={GAMEPLAY_MODES}
          available={gameplays}
          value={filters.gameplay}
          onChange={(v) => onChange({ ...filters, gameplay: v })}
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
  )
}

interface ModeFilterGroupProps {
  label: string
  shortLabel: string
  modes: Record<string, { icon: string; label: string }>
  available: string[]
  value: string
  onChange: (next: string) => void
}

function ModeFilterGroup({ label, shortLabel, modes, available, value, onChange }: ModeFilterGroupProps) {
  return (
    <div className="server-filter-group" role="radiogroup" aria-label={`Filter by ${label.toLowerCase()}`}>
      <span className="server-filter-label" aria-hidden="true">{shortLabel}</span>
      <button
        type="button"
        className={`mode-filter-btn ${value === '' ? 'active' : ''}`}
        onClick={() => onChange('')}
        title={`All ${label}`}
      >
        All
      </button>
      {available.map((id) => {
        const mode = modes[id]
        if (!mode) return null
        return (
          <button
            type="button"
            key={id}
            className={`mode-filter-btn icon-btn ${value === id ? 'active' : ''}`}
            onClick={() => onChange(id)}
            title={`${label}: ${mode.label}`}
            aria-label={`${label}: ${mode.label}`}
          >
            <img src={mode.icon} alt="" />
          </button>
        )
      })}
    </div>
  )
}
