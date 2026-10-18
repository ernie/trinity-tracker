import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useDebouncedValue } from "../hooks/useDebouncedValue";
import { useAuth } from "../hooks/useAuth";
import type { PlayerProfile } from "../types";

// Apple keyboards label the modifier ⌘; everywhere else it's Ctrl.
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

const ROUTE_ITEMS = [
  { path: "/", label: "Servers", hint: "Live server browser" },
  {
    path: "/players",
    label: "Players",
    hint: "Search players and see their stats",
  },
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
  { path: "/docs/play#q3-basics", label: "Quake III basics" },
  { path: "/docs/play#accounts", label: "Trinity accounts" },
  { path: "/docs/play#voice-chat", label: "Voice chat" },
  { path: "/docs/play#vr-tracking", label: "1:1 head and weapon tracking" },
  { path: "/docs/play#vr", label: "VR-specific features" },
  { path: "/docs/play#hud", label: "HUD" },
  { path: "/docs/play#scoreboard", label: "Scoreboard" },
  { path: "/docs/play#modes", label: "Game modes" },
  { path: "/docs/play#grapple", label: "Grapple" },
  { path: "/docs/play#spectating-demos", label: "Spectating and demos" },
  { path: "/docs/play#feedback", label: "Visual & audio feedback" },
  { path: "/docs/play#forced-models", label: "Forced enemy / team models" },
  { path: "/docs/account", label: "Account", hint: "!claim · log in" },
  { path: "/docs/account#claim", label: "Create your account" },
  { path: "/docs/account#authentication", label: "Log in" },
  { path: "/docs/customize", label: "Customize", hint: "autoexec · modes" },
  { path: "/docs/customize#where-settings-live", label: "Where settings live" },
  { path: "/docs/customize#modes-note", label: "Game modes" },
  { path: "/docs/admin", label: "Server admin", hint: "Collector setup" },
  { path: "/docs/admin#set-up-a-collector", label: "Set up a collector" },
  { path: "/docs/admin#required-server-cvars", label: "Required server cvars" },
  { path: "/docs/reference", label: "Reference", hint: "cvars · glossary" },
  { path: "/docs/reference#glossary", label: "Glossary" },
  {
    path: "/docs/customize#color-codes",
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

  const [prevOpen, setPrevOpen] = useState(open);
  if (open !== prevOpen) {
    setPrevOpen(open);
    if (open) {
      setQuery("");
      setHighlightIdx(0);
      setPlayers([]);
    }
  }

  // rAF waits for the input to mount.
  useEffect(() => {
    if (open) requestAnimationFrame(() => inputRef.current?.focus());
  }, [open]);

  // Short queries skip the fetch; `items` ignores `players` until the
  // query is long enough again.
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

  const items = useMemo<CommandItem[]>(() => {
    const out: CommandItem[] = [];

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

    // Gated on the live query because `players` can be stale.
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

  // The list can shrink under the stored highlight as the query narrows.
  const maxIdx = Math.max(0, items.length - 1);
  const safeHighlight = Math.min(highlightIdx, maxIdx);

  useEffect(() => {
    const el = listRef.current?.querySelector<HTMLElement>(
      `[data-idx="${safeHighlight}"]`,
    );
    if (el) el.scrollIntoView({ block: "nearest" });
  }, [safeHighlight]);

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
