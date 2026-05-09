import { useState, useEffect, useRef } from 'react'
import { useSearchParams } from 'react-router-dom'
import { MatchCard, formatGameType } from './MatchCard'
import { SourceFilter } from './SourceFilter'
import { ModeFilterGroup } from './ServerFilters'
import { MOVEMENT_MODES, GAMEPLAY_MODES } from './ServerCard'
import { GAME_TYPES, isGameTypeFilter, type GameTypeFilter } from '../constants/labels'
import { useLiveData } from '../contexts/LiveDataContext'
import type { MatchSummary } from '../types'

// Target page size — actual fetch limit rounds the *total displayed
// count* (existing + this batch) to the nearest multiple of the grid
// column count, so the bottom row is always full and any orphans left
// behind by a viewport resize get healed by the next load.
const PAGE_SIZE = 10
// Mirrors the .matches-browser-list grid: minmax(350px, 1fr) with 20px
// gap. Kept in sync by hand — if the CSS changes, update here too.
const GRID_MIN_WIDTH = 350
const GRID_GAP = 20

function columnsForWidth(width: number): number {
  if (width <= 0) return 0
  return Math.max(1, Math.floor((width + GRID_GAP) / (GRID_MIN_WIDTH + GRID_GAP)))
}

// How many cards to fetch next so the post-load total lands on a clean
// row. Same math handles initial load (currentCount=0 → "round PAGE_SIZE
// to nearest multiple") and incremental loads after a resize-induced
// orphan (currentCount=9, cols=4, PAGE_SIZE=10 → desired=19 → nearest
// multiple of 4 is 20 → batch=11).
function nextBatchSize(currentCount: number, columns: number): number {
  if (columns <= 0) return PAGE_SIZE
  const desired = currentCount + PAGE_SIZE
  const nearest = Math.round(desired / columns) * columns
  const batch = nearest - currentCount
  // Safety net: when current is already at the rounded total (only
  // possible in pathological cases with cols > PAGE_SIZE), still load
  // one full row so a "Load more" click does something visible.
  return batch > 0 ? batch : columns
}

const MOVEMENT_KEYS = Object.keys(MOVEMENT_MODES)
const GAMEPLAY_KEYS = Object.keys(GAMEPLAY_MODES)

function parseDateTimeLocal(value: string): Date | null {
  if (!value) return null
  const date = new Date(value)
  return isNaN(date.getTime()) ? null : date
}

export function MatchesPage() {

  const { showPlayer } = useLiveData()
  const [searchParams, setSearchParams] = useSearchParams()

  // Read initial state from URL
  const [gameType, setGameType] = useState<GameTypeFilter>(() => {
    const gt = searchParams.get('game_type')
    return gt && isGameTypeFilter(gt) ? gt : 'all'
  })
  const [startDate, setStartDate] = useState<string>(() => searchParams.get('start_date') || '')
  const [endDate, setEndDate] = useState<string>(() => searchParams.get('end_date') || '')
  const [includeBotOnly, setIncludeBotOnly] = useState<boolean>(() =>
    searchParams.get('include_bot_only') === 'true'
  )
  const [source, setSource] = useState<string>(() => searchParams.get('source') || '')
  const [movement, setMovement] = useState<string>(() => {
    const m = searchParams.get('movement') || ''
    return m && m in MOVEMENT_MODES ? m : ''
  })
  const [gameplay, setGameplay] = useState<string>(() => {
    const g = searchParams.get('gameplay') || ''
    return g && g in GAMEPLAY_MODES ? g : ''
  })

  const [matches, setMatches] = useState<MatchSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [hasMore, setHasMore] = useState(true)

  // Grid column count, observed from the always-rendered content
  // container. `columns` is a state to gate the initial fetch (we
  // don't fetch until we know how many cards make a clean row);
  // `columnsRef` mirrors it so loadMore reads the live value without
  // pulling it into the callback's closure deps.
  const contentRef = useRef<HTMLDivElement>(null)
  const columnsRef = useRef(0)
  const [columns, setColumns] = useState(0)

  useEffect(() => {
    const el = contentRef.current
    if (!el) return
    const measure = () => {
      const next = columnsForWidth(el.clientWidth)
      if (next === columnsRef.current) return
      columnsRef.current = next
      setColumns(next)
    }
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  // Update URL when filters change
  useEffect(() => {
    const params = new URLSearchParams()
    if (gameType !== 'all') params.set('game_type', gameType)
    if (startDate) params.set('start_date', startDate)
    if (endDate) params.set('end_date', endDate)
    if (includeBotOnly) params.set('include_bot_only', 'true')
    if (source) params.set('source', source)
    if (movement) params.set('movement', movement)
    if (gameplay) params.set('gameplay', gameplay)
    setSearchParams(params, { replace: true })
  }, [gameType, startDate, endDate, includeBotOnly, source, movement, gameplay, setSearchParams])

  // Fetch matches when filters change. Waits for the column count so
  // the very first batch is also row-aligned (an over-fetch on initial
  // load would visibly orphan a card at the bottom). Subsequent column
  // changes (viewport resize) intentionally don't refetch — that would
  // wipe the user's loaded batch; the next loadMore picks up the new
  // width via columnsRef.
  const measured = columns > 0
  useEffect(() => {
    if (!measured) return
    // Initial fetch — currentCount is 0 since we're starting fresh.
    const limit = nextBatchSize(0, columnsRef.current)
    async function fetchMatches() {
      try {
        setLoading(true)
        setMatches([])
        setHasMore(true)

        const params = new URLSearchParams()
        params.set('limit', limit.toString())
        if (gameType !== 'all') params.set('game_type', gameType)
        if (includeBotOnly) params.set('include_bot_only', 'true')
        if (source) params.set('source', source)
        if (movement) params.set('movement', movement)
        if (gameplay) params.set('gameplay', gameplay)

        // Convert datetime-local to RFC3339
        if (startDate) {
          const date = parseDateTimeLocal(startDate)
          if (date) params.set('start_date', date.toISOString())
        }
        if (endDate) {
          const date = parseDateTimeLocal(endDate)
          if (date) params.set('end_date', date.toISOString())
        }

        const res = await fetch(`/api/matches?${params.toString()}`)
        if (res.ok) {
          const data = await res.json()
          const fetchedMatches = data ?? []
          setMatches(fetchedMatches)
          setHasMore(fetchedMatches.length === limit)
        }
      } catch (e) {
        console.error('Failed to fetch matches:', e)
      } finally {
        setLoading(false)
      }
    }

    fetchMatches()
  }, [gameType, startDate, endDate, includeBotOnly, source, movement, gameplay, measured])

  const loadMore = async () => {
    if (loadingMore || !hasMore || matches.length === 0) return

    const lastMatchId = matches[matches.length - 1].id
    // Use the live `matches.length` so the next batch heals any orphan
    // row left by a viewport resize since the last fetch.
    const limit = nextBatchSize(matches.length, columnsRef.current)
    try {
      setLoadingMore(true)

      const params = new URLSearchParams()
      params.set('limit', limit.toString())
      params.set('before', lastMatchId.toString())
      if (gameType !== 'all') params.set('game_type', gameType)
      if (includeBotOnly) params.set('include_bot_only', 'true')
      if (source) params.set('source', source)
      if (movement) params.set('movement', movement)
      if (gameplay) params.set('gameplay', gameplay)
      if (startDate) {
        const date = parseDateTimeLocal(startDate)
        if (date) params.set('start_date', date.toISOString())
      }
      if (endDate) {
        const date = parseDateTimeLocal(endDate)
        if (date) params.set('end_date', date.toISOString())
      }

      const res = await fetch(`/api/matches?${params.toString()}`)
      if (res.ok) {
        const data = await res.json()
        const newMatches = data ?? []
        setMatches(prev => [...prev, ...newMatches])
        setHasMore(newMatches.length === limit)
      }
    } catch (e) {
      console.error('Failed to fetch more matches:', e)
    } finally {
      setLoadingMore(false)
    }
  }

  const clearFilters = () => {
    setGameType('all')
    setStartDate('')
    setEndDate('')
    setIncludeBotOnly(false)
    setSource('')
    setMovement('')
    setGameplay('')
  }

  const hasActiveFilters =
    gameType !== 'all' || startDate || endDate || includeBotOnly || source || movement || gameplay

  return (
    <div className="matches-page">
      <div className="match-filters">
        <div className="game-type-selector">
          <button
            key="all"
            className={`game-type-btn ${gameType === 'all' ? 'active' : ''}`}
            onClick={() => setGameType('all')}
          >
            All
          </button>
          {GAME_TYPES.map((gt) => (
            <button
              key={gt}
              className={`game-type-btn ${gameType === gt ? 'active' : ''}`}
              onClick={() => setGameType(gt)}
            >
              {formatGameType(gt)}
            </button>
          ))}
        </div>

        <div className="match-mode-filters">
          <ModeFilterGroup
            label="Movement"
            shortLabel="Movement:"
            modes={MOVEMENT_MODES}
            available={MOVEMENT_KEYS}
            value={movement}
            onChange={setMovement}
          />
          <ModeFilterGroup
            label="Gameplay"
            shortLabel="Gameplay:"
            modes={GAMEPLAY_MODES}
            available={GAMEPLAY_KEYS}
            value={gameplay}
            onChange={setGameplay}
          />
        </div>

        <div className="date-range-filters">
          <label className="date-filter">
            <span>From:</span>
            <input
              type="datetime-local"
              value={startDate}
              onChange={(e) => setStartDate(e.target.value)}
            />
          </label>
          <label className="date-filter">
            <span>To:</span>
            <input
              type="datetime-local"
              value={endDate}
              onChange={(e) => setEndDate(e.target.value)}
            />
          </label>
          <label className="toggle-filter">
            <input
              type="checkbox"
              checked={includeBotOnly}
              onChange={(e) => setIncludeBotOnly(e.target.checked)}
            />
            <span className="toggle-filter__switch" aria-hidden />
            <span className="toggle-filter__label">Include bot-only</span>
          </label>
          <SourceFilter value={source} onChange={setSource} />
          {hasActiveFilters && (
            <button className="clear-filters-btn" onClick={clearFilters}>
              Clear Filters
            </button>
          )}
        </div>
      </div>

      <div className="matches-content" ref={contentRef}>
        {loading ? (
          <div className="stats-loading">Loading matches...</div>
        ) : matches.length === 0 ? (
          <div className="matches-empty">
            No matches found{hasActiveFilters ? ' for the selected filters' : ''}
          </div>
        ) : (
          <>
            <div className="matches-list matches-browser-list">
              {matches.map((match) => (
                <MatchCard
                  key={match.id}
                  match={match}
                  onPlayerClick={showPlayer}
                  showPermalink
                />
              ))}
            </div>
            {hasMore && (
              <div className="load-more-container">
                <button
                  className="load-more-btn"
                  onClick={loadMore}
                  disabled={loadingMore}
                >
                  {loadingMore ? 'Loading...' : 'Load More'}
                </button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
