import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import { useAuth } from '../hooks/useAuth'
import type { PlayerProfile } from '../types'

interface CommandPaletteProps {
  open: boolean
  onClose: () => void
}

interface CommandItem {
  id: string
  group: string
  label: string
  hint?: string
  action: () => void
}

// Static items always available — fast keyboard-only navigation around
// the app and into specific docs sections.
const ROUTE_ITEMS = [
  { path: '/', label: 'Servers', hint: 'Live server browser' },
  { path: '/players', label: 'Players', hint: 'Search players & view stats' },
  { path: '/matches', label: 'Matches', hint: 'Browse match history' },
  { path: '/leaderboard', label: 'Leaderboard', hint: 'Top players by category' },
  { path: '/account', label: 'Account', hint: 'Your account settings' },
]

const DOCS_ITEMS = [
  { path: '/docs/getting-started', label: 'Getting started', hint: 'Install · account · updates' },
  { path: '/docs/getting-started#install-trinity', label: 'Install Trinity' },
  { path: '/docs/getting-started#your-account', label: 'Your account' },
  { path: '/docs/getting-started#automatic-updates', label: 'Automatic updates' },
  { path: '/docs/getting-started#configuration', label: 'Configuration' },
  { path: '/docs/features', label: 'Features', hint: 'Trinity features · modes' },
  { path: '/docs/features#trinity-features', label: 'Trinity features' },
  { path: '/docs/features#gameplay-modes', label: 'Gameplay modes' },
  { path: '/docs/features#movement-modes', label: 'Movement modes' },
  { path: '/docs/features#authentication', label: 'Authentication' },
  { path: '/docs/server-admin', label: 'Server admin', hint: 'Hub setup & cvars' },
  { path: '/docs/server-admin#contributing-stats', label: 'Contributing stats' },
  { path: '/docs/server-admin#server-cvars', label: 'Server cvars' },
  { path: '/docs/server-admin#trinity-handshake', label: 'Trinity handshake' },
  { path: '/docs/credits', label: 'Credits' },
]

// Substring + token-prefix scoring. Returns 0 for no match.
function score(query: string, target: string): number {
  if (!query) return 1
  const q = query.toLowerCase()
  const t = target.toLowerCase()
  if (t === q) return 1000
  if (t.startsWith(q)) return 500
  // Token prefix: any whitespace-separated word starts with the query.
  for (const tok of t.split(/\s+/)) if (tok.startsWith(q)) return 250
  if (t.includes(q)) return 100
  return 0
}

export function CommandPalette({ open, onClose }: CommandPaletteProps) {
  const navigate = useNavigate()
  const { auth } = useAuth()
  const [query, setQuery] = useState('')
  const [highlightIdx, setHighlightIdx] = useState(0)
  const [players, setPlayers] = useState<PlayerProfile[]>([])
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const debounced = useDebouncedValue(query, 180)

  // Reset state when the palette opens.
  useEffect(() => {
    if (open) {
      setQuery('')
      setHighlightIdx(0)
      setPlayers([])
      // Focus runs after the input mounts.
      requestAnimationFrame(() => inputRef.current?.focus())
    }
  }, [open])

  // Live player search — only fires when palette is open and query is
  // long enough to be meaningful.
  useEffect(() => {
    if (!open || debounced.trim().length < 2) {
      setPlayers([])
      return
    }
    const ctrl = new AbortController()
    const headers: HeadersInit = {}
    if (auth.token) headers['Authorization'] = `Bearer ${auth.token}`
    fetch(`/api/players?search=${encodeURIComponent(debounced)}&limit=8`, { headers, signal: ctrl.signal })
      .then((r) => (r.ok ? r.json() : []))
      .then((data: PlayerProfile[]) => setPlayers(data ?? []))
      .catch(() => { /* aborted or network error */ })
    return () => ctrl.abort()
  }, [debounced, open, auth.token])

  // Build the result list: static items filtered by query, plus live
  // player results. Each item belongs to a group for visual sectioning.
  const items = useMemo<CommandItem[]>(() => {
    const out: CommandItem[] = []

    // Routes
    for (const r of ROUTE_ITEMS) {
      if (score(query, r.label) === 0 && score(query, r.hint ?? '') === 0) continue
      out.push({
        id: `route:${r.path}`,
        group: 'Go to',
        label: r.label,
        hint: r.hint,
        action: () => { navigate(r.path); onClose() },
      })
    }

    // Docs sections
    for (const d of DOCS_ITEMS) {
      if (score(query, d.label) === 0 && score(query, d.hint ?? '') === 0) continue
      out.push({
        id: `doc:${d.path}`,
        group: 'Docs',
        label: d.label,
        hint: d.hint,
        action: () => { navigate(d.path); onClose() },
      })
    }

    // Players
    for (const p of players) {
      out.push({
        id: `player:${p.id}`,
        group: 'Players',
        label: p.clean_name || `Player #${p.id}`,
        hint: p.last_seen ? `Last seen ${new Date(p.last_seen).toLocaleDateString()}` : undefined,
        action: () => { navigate(`/players/${p.id}`); onClose() },
      })
    }

    return out
    // navigate / onClose are stable refs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, players])

  // Reset highlight when items change so we never point past the end.
  useEffect(() => {
    setHighlightIdx((i) => Math.min(i, Math.max(0, items.length - 1)))
  }, [items.length])

  // Scroll the highlighted item into view as the user keys around.
  useEffect(() => {
    const el = listRef.current?.querySelector<HTMLElement>(`[data-idx="${highlightIdx}"]`)
    if (el) el.scrollIntoView({ block: 'nearest' })
  }, [highlightIdx])

  // Group items by their group label, preserving insertion order.
  const groups = useMemo(() => {
    const map = new Map<string, { idx: number; item: CommandItem }[]>()
    items.forEach((item, idx) => {
      const list = map.get(item.group) ?? []
      list.push({ idx, item })
      map.set(item.group, list)
    })
    return Array.from(map.entries())
  }, [items])

  if (!open) return null

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setHighlightIdx((i) => Math.min(i + 1, items.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setHighlightIdx((i) => Math.max(i - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      items[highlightIdx]?.action()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
    }
  }

  return (
    <div className="cmdk-overlay" onClick={onClose}>
      <div
        className="cmdk-palette"
        role="dialog"
        aria-label="Command palette"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="cmdk-input-wrap">
          <span className="cmdk-prompt" aria-hidden="true">⌘K</span>
          <input
            ref={inputRef}
            className="cmdk-input"
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Search players, matches, docs…"
            spellCheck={false}
            autoComplete="off"
            aria-autocomplete="list"
          />
          <kbd className="cmdk-kbd" aria-hidden="true">esc</kbd>
        </div>
        <div className="cmdk-list" ref={listRef} role="listbox">
          {items.length === 0 ? (
            <div className="cmdk-empty">
              {query.trim().length < 2 && players.length === 0 ? 'Type to search…' : 'No matches'}
            </div>
          ) : groups.map(([group, list]) => (
            <div key={group} className="cmdk-group">
              <div className="cmdk-group__title">{group}</div>
              {list.map(({ idx, item }) => (
                <button
                  key={item.id}
                  type="button"
                  data-idx={idx}
                  className={`cmdk-item ${idx === highlightIdx ? 'cmdk-item--active' : ''}`}
                  onMouseEnter={() => setHighlightIdx(idx)}
                  onClick={item.action}
                  role="option"
                  aria-selected={idx === highlightIdx}
                >
                  <span className="cmdk-item__label">{item.label}</span>
                  {item.hint && <span className="cmdk-item__hint">{item.hint}</span>}
                </button>
              ))}
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
