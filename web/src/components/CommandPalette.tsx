import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useDebouncedValue } from "../hooks/useDebouncedValue";
import { useAuth } from "../hooks/useAuth";
import type { PlayerProfile } from "../types";

// On Mac the canonical key glyph is ⌘ (the actual key cap symbol);
// elsewhere we write "Ctrl" so Linux/Windows users see the right
// modifier name. SSR-safe via the typeof guard.
const CMD_LABEL =
  typeof navigator !== "undefined" &&
  /Mac|iPhone|iPad|iPod/i.test(navigator.userAgent)
    ? "⌘"
    : "Ctrl";

interface CommandPaletteProps {
  open: boolean;
  onClose: () => void;
}

interface CommandItem {
  id: string;
  group: string;
  label: string;
  hint?: string;
  action: () => void;
}

// Static items always available — fast keyboard-only navigation around
// the app and into specific docs sections.
const ROUTE_ITEMS = [
  { path: "/", label: "Servers", hint: "Live server browser" },
  { path: "/players", label: "Players", hint: "Search players & view stats" },
  { path: "/matches", label: "Matches", hint: "Browse match history" },
  {
    path: "/leaderboard",
    label: "Leaderboard",
    hint: "Top players by category",
  },
  { path: "/account", label: "Account", hint: "Your account settings" },
];

const DOCS_ITEMS = [
  { path: "/docs", label: "Welcome", hint: "Start here" },
  {
    path: "/docs/install",
    label: "Install",
    hint: "Download · assets · updates",
  },
  {
    path: "/docs/play",
    label: "Play",
    hint: "Player's guide · what Trinity adds",
  },
  { path: "/docs/play#q3-basics", label: "Quake 3 basics" },
  { path: "/docs/play#accounts", label: "Trinity accounts" },
  { path: "/docs/play#voice-chat", label: "Voice chat" },
  { path: "/docs/play#vr-tracking", label: "1:1 head & weapon tracking" },
  { path: "/docs/play#vr", label: "VR features" },
  { path: "/docs/play#hud", label: "HUD" },
  { path: "/docs/play#scoreboard", label: "Scoreboard" },
  { path: "/docs/play#modes", label: "Movement & gameplay variants" },
  { path: "/docs/play#spectating-demos", label: "Spectating & demos" },
  { path: "/docs/play#feedback", label: "Visual & audio feedback" },
  { path: "/docs/play#forced-models", label: "Forced enemy / team models" },
  { path: "/docs/account", label: "Account", hint: "!claim · authentication" },
  { path: "/docs/account#your-account", label: "Your account" },
  { path: "/docs/account#authentication", label: "Authentication" },
  { path: "/docs/customize", label: "Customize", hint: "autoexec · modes" },
  { path: "/docs/customize#configuration", label: "Configuration" },
  { path: "/docs/customize#gameplay-modes", label: "Gameplay modes" },
  { path: "/docs/customize#movement-modes", label: "Movement modes" },
  { path: "/docs/admin", label: "Server admin", hint: "Hub setup" },
  { path: "/docs/admin#set-up-a-collector", label: "Set up a collector" },
  { path: "/docs/admin#required-server-cvars", label: "Required server cvars" },
  { path: "/docs/reference", label: "Reference", hint: "cvars · glossary" },
  { path: "/docs/reference#glossary", label: "Glossary" },
  {
    path: "/docs/reference#color-codes",
    label: "Color codes",
    hint: "slider · cvars · chat/names",
  },
  { path: "/docs/reference#player-cvars", label: "Player cvars" },
  { path: "/docs/reference#vr-cvars", label: "VR cvars" },
  { path: "/docs/reference#server-cvars", label: "Server cvars" },
];

// Substring + token-prefix scoring. Returns 0 for no match.
function score(query: string, target: string): number {
  if (!query) return 1;
  const q = query.toLowerCase();
  const t = target.toLowerCase();
  if (t === q) return 1000;
  if (t.startsWith(q)) return 500;
  // Token prefix: any whitespace-separated word starts with the query.
  for (const tok of t.split(/\s+/)) if (tok.startsWith(q)) return 250;
  if (t.includes(q)) return 100;
  return 0;
}

export function CommandPalette({ open, onClose }: CommandPaletteProps) {
  const navigate = useNavigate();
  const { auth } = useAuth();
  const [query, setQuery] = useState("");
  const [highlightIdx, setHighlightIdx] = useState(0);
  const [players, setPlayers] = useState<PlayerProfile[]>([]);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const debounced = useDebouncedValue(query, 180);

  // Reset palette state on the open transition. Adjusting state during
  // render so there's no extra commit cycle.
  const [prevOpen, setPrevOpen] = useState(open);
  if (open !== prevOpen) {
    setPrevOpen(open);
    if (open) {
      setQuery("");
      setHighlightIdx(0);
      setPlayers([]);
    }
  }

  // Focus the input on the open transition (side-effect, must stay in
  // an effect — rAF schedules after the input mounts).
  useEffect(() => {
    if (open) requestAnimationFrame(() => inputRef.current?.focus());
  }, [open]);

  // Live player search — only fires when palette is open and query is
  // long enough to be meaningful. The display gate at the consumer
  // (items useMemo below also reads `players`) means stale state from
  // a too-short query is overwritten by the next fetch when the query
  // grows back; no clear needed in the early-return.
  useEffect(() => {
    if (!open || debounced.trim().length < 2) return;
    const ctrl = new AbortController();
    fetch(`/api/players?search=${encodeURIComponent(debounced)}&limit=8`, {
      signal: ctrl.signal,
    })
      .then((r) => (r.ok ? r.json() : []))
      .then((data: PlayerProfile[]) => setPlayers(data ?? []))
      .catch(() => {
        /* aborted or network error */
      });
    return () => ctrl.abort();
  }, [debounced, open, auth.isAuthenticated]);

  // Build the result list: static items filtered by query, plus live
  // player results. Each item belongs to a group for visual sectioning.
  const items = useMemo<CommandItem[]>(() => {
    const out: CommandItem[] = [];

    // Routes
    for (const r of ROUTE_ITEMS) {
      if (score(query, r.label) === 0 && score(query, r.hint ?? "") === 0)
        continue;
      out.push({
        id: `route:${r.path}`,
        group: "Go to",
        label: r.label,
        hint: r.hint,
        action: () => {
          navigate(r.path);
          onClose();
        },
      });
    }

    // Docs sections
    for (const d of DOCS_ITEMS) {
      if (score(query, d.label) === 0 && score(query, d.hint ?? "") === 0)
        continue;
      out.push({
        id: `doc:${d.path}`,
        group: "Docs",
        label: d.label,
        hint: d.hint,
        action: () => {
          navigate(d.path);
          onClose();
        },
      });
    }

    // Players — gate on the live query, not the (potentially stale)
    // `players` array. When the query is too short, skip the section
    // entirely; the next ≥2-char query overwrites `players`.
    if (query.trim().length >= 2) {
      for (const p of players) {
        out.push({
          id: `player:${p.id}`,
          group: "Players",
          label: p.clean_name || `Player #${p.id}`,
          hint: p.last_seen
            ? `Last seen ${new Date(p.last_seen).toLocaleDateString()}`
            : undefined,
          action: () => {
            navigate(`/players/${p.id}`);
            onClose();
          },
        });
      }
    }

    return out;
    // navigate / onClose are stable refs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, players]);

  // Clamp the highlight to the live items range so it never points
  // past the end after the list shrinks (e.g. user types a more
  // restrictive query). Derived rather than stored — keyboard handlers
  // below pivot off this so the raw `highlightIdx` state never goes
  // out of bounds in practice.
  const maxIdx = Math.max(0, items.length - 1);
  const safeHighlight = Math.min(highlightIdx, maxIdx);

  // Scroll the highlighted item into view as the user keys around.
  useEffect(() => {
    const el = listRef.current?.querySelector<HTMLElement>(
      `[data-idx="${safeHighlight}"]`,
    );
    if (el) el.scrollIntoView({ block: "nearest" });
  }, [safeHighlight]);

  // Group items by their group label, preserving insertion order.
  const groups = useMemo(() => {
    const map = new Map<string, { idx: number; item: CommandItem }[]>();
    items.forEach((item, idx) => {
      const list = map.get(item.group) ?? [];
      list.push({ idx, item });
      map.set(item.group, list);
    });
    return Array.from(map.entries());
  }, [items]);

  if (!open) return null;

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setHighlightIdx(Math.min(safeHighlight + 1, maxIdx));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setHighlightIdx(Math.max(safeHighlight - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      items[safeHighlight]?.action();
    } else if (e.key === "Escape") {
      e.preventDefault();
      onClose();
    }
  };

  return (
    <div className="cmdk-overlay" onClick={onClose}>
      <div
        className="cmdk-palette"
        role="dialog"
        aria-label="Command palette"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="cmdk-input-wrap">
          <span className="cmdk-prompt" aria-hidden="true">
            {CMD_LABEL}K
          </span>
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
          <kbd className="cmdk-kbd" aria-hidden="true">
            esc
          </kbd>
        </div>
        <div className="cmdk-list" ref={listRef} role="listbox">
          {items.length === 0 ? (
            <div className="cmdk-empty">
              {query.trim().length < 2 && players.length === 0
                ? "Type to search…"
                : "No matches"}
            </div>
          ) : (
            groups.map(([group, list]) => (
              <div key={group} className="cmdk-group">
                <div className="cmdk-group__title">{group}</div>
                {list.map(({ idx, item }) => (
                  <button
                    key={item.id}
                    type="button"
                    data-idx={idx}
                    className={`cmdk-item ${idx === safeHighlight ? "cmdk-item--active" : ""}`}
                    onMouseEnter={() => setHighlightIdx(idx)}
                    onClick={item.action}
                    role="option"
                    aria-selected={idx === safeHighlight}
                  >
                    <span className="cmdk-item__label">{item.label}</span>
                    {item.hint && (
                      <span className="cmdk-item__hint">{item.hint}</span>
                    )}
                  </button>
                ))}
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
}
