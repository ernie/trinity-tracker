import { useState, useCallback, useEffect } from "react";
import { Link } from "react-router-dom";
import { useDebouncedValue } from "../../hooks/useDebouncedValue";
import { BotBadge } from "../BotBadge";
import { ColoredText } from "../ColoredText";
import { PlayerBadge } from "../PlayerBadge";
import { PlayerPortrait } from "../PlayerPortrait";
import { formatDate, formatDuration } from "../../utils/formatters";
import { displayPlayerName, stripVRPrefix } from "../../utils";
import type { PlayerProfile, PlayerGUID } from "../../types";
import { apiFetch } from "../../authFetch";

const PAGE_SIZE = 24;

const SORTS = [
  { key: "last_seen", label: "Last seen" },
  { key: "first_seen", label: "First seen" },
  { key: "playtime", label: "Playtime" },
  { key: "name", label: "Name" },
] as const;
type SortKey = (typeof SORTS)[number]["key"];

// Recency and playtime read best newest/biggest-first; name is the one
// criterion humans expect A→Z. Mirrors the API's default-direction rule.
function defaultDir(sort: SortKey): "asc" | "desc" {
  return sort === "name" ? "asc" : "desc";
}

// Shared card between the search results and the browse grid. Same
// structure as PlayersPage's cards — .player-card is a centered flex
// column, so name/meta must be direct children (a wrapper div would
// collapse them onto one inline line).
function PlayerCardButton({
  player,
  onSelect,
}: {
  player: PlayerProfile;
  onSelect: (p: PlayerProfile) => void;
}) {
  return (
    <button
      type="button"
      className="player-card"
      onClick={() => onSelect(player)}
    >
      <span className="player-card__avatar">
        <PlayerPortrait model={player.model} size="lg" />
        {player.is_bot ? (
          <BotBadge isBot skill={5} size="sm" />
        ) : (
          <PlayerBadge
            isVerified={player.is_verified}
            isAdmin={player.is_admin}
            isVR={player.is_vr}
            size="sm"
          />
        )}
      </span>
      <span className="player-card__name">
        <ColoredText text={displayPlayerName(player)} />
      </span>
      <span className="player-card__meta">
        Last seen {formatDate(player.last_seen)}
      </span>
      {player.total_playtime_seconds > 0 && (
        <span className="player-card__meta">
          {formatDuration(player.total_playtime_seconds)} played
        </span>
      )}
    </button>
  );
}

export function AdminPlayers() {
  // Player search (target selection) — fires automatically as the admin types.
  const [searchQuery, setSearchQuery] = useState("");
  const [searchResults, setSearchResults] = useState<PlayerProfile[]>([]);
  const debouncedSearchQuery = useDebouncedValue(searchQuery, 200);

  // Selected target player
  const [selected, setSelected] = useState<PlayerProfile | null>(null);
  const [guids, setGuids] = useState<PlayerGUID[]>([]);
  const [loadingGuids, setLoadingGuids] = useState(false);

  // Merge state
  const [mergeQuery, setMergeQuery] = useState("");
  const [mergeResults, setMergeResults] = useState<PlayerProfile[]>([]);
  const debouncedMergeQuery = useDebouncedValue(mergeQuery, 200);
  const [merging, setMerging] = useState(false);
  const [splitting, setSplitting] = useState<number | null>(null);
  const [error, setError] = useState("");

  // Browse list — shown when neither a search nor a selection is active,
  // so the page is useful before the admin knows who they're looking for.
  // Load-more (not paged): earlier results stay in the DOM so the admin
  // can scan up and down and use find-in-page across everything loaded.
  const [sort, setSort] = useState<SortKey>("last_seen");
  const [dir, setDir] = useState<"asc" | "desc">("desc");
  const [includeBots, setIncludeBots] = useState(false);
  const [list, setList] = useState<PlayerProfile[] | null>(null);
  const [total, setTotal] = useState(0);
  const [loadingMore, setLoadingMore] = useState(false);

  useEffect(() => {
    const ctrl = new AbortController();
    const params = new URLSearchParams();
    params.set("limit", String(PAGE_SIZE));
    params.set("sort", sort);
    params.set("dir", dir);
    if (includeBots) params.set("include_bots", "true");
    apiFetch(`/api/players?${params.toString()}`, { signal: ctrl.signal })
      .then((res) => (res.ok ? res.json() : null))
      .then(
        (data: { players: PlayerProfile[] | null; total: number } | null) => {
          if (!data) return;
          setList(data.players ?? []);
          setTotal(data.total);
        },
      )
      .catch(() => {
        /* aborted or network error */
      });
    return () => ctrl.abort();
  }, [sort, dir, includeBots]);

  const loadMore = async () => {
    if (loadingMore || list === null) return;
    setLoadingMore(true);
    try {
      const params = new URLSearchParams();
      params.set("limit", String(PAGE_SIZE));
      params.set("offset", String(list.length));
      params.set("sort", sort);
      params.set("dir", dir);
      if (includeBots) params.set("include_bots", "true");
      const res = await apiFetch(`/api/players?${params.toString()}`);
      if (res.ok) {
        const data: { players: PlayerProfile[] | null; total: number } =
          await res.json();
        // Offset pagination can re-serve a row if the sort order shifted
        // between fetches (e.g. last_seen ticked); drop already-shown ids
        // so React keys stay unique.
        setList((prev) => {
          const seen = new Set((prev ?? []).map((p) => p.id));
          const fresh = (data.players ?? []).filter((p) => !seen.has(p.id));
          return [...(prev ?? []), ...fresh];
        });
        setTotal(data.total);
      }
    } finally {
      setLoadingMore(false);
    }
  };

  const changeSort = (key: SortKey) => {
    setSort(key);
    setDir(defaultDir(key));
  };

  useEffect(() => {
    if (debouncedSearchQuery.trim().length < 2) return;
    const ctrl = new AbortController();
    apiFetch(
      `/api/players?search=${encodeURIComponent(debouncedSearchQuery)}&limit=10`,
      {
        signal: ctrl.signal,
      },
    )
      .then((res) => (res.ok ? res.json() : []))
      .then((data: PlayerProfile[]) => setSearchResults(data ?? []))
      .catch(() => {
        /* aborted or network error */
      });
    return () => ctrl.abort();
  }, [debouncedSearchQuery]);

  // Hide stored results once the query is too short — store keeps the
  // last fetch's data so a fresh ≥2-char query overwrites cleanly.
  const displaySearchResults =
    debouncedSearchQuery.trim().length >= 2 ? searchResults : [];

  const fetchGuids = useCallback((playerId: number) => {
    setLoadingGuids(true);
    apiFetch(`/api/players/${playerId}/guids`)
      .then((res) => (res.ok ? res.json() : []))
      .then((data) => setGuids(data || []))
      .catch(() => setGuids([]))
      .finally(() => setLoadingGuids(false));
  }, []);

  // GUIDs load on the selection *event* rather than an effect synced to
  // `selected` — there's no render-time state to reconcile, just a fetch
  // that belongs to the click.
  const selectPlayer = (p: PlayerProfile) => {
    setSelected(p);
    setSearchResults([]);
    setSearchQuery("");
    setMergeQuery("");
    setMergeResults([]);
    setError("");
    fetchGuids(p.id);
  };

  const clearSelection = () => {
    setSelected(null);
    setGuids([]);
    setError("");
  };

  useEffect(() => {
    if (!selected || debouncedMergeQuery.trim().length < 2) return;
    const ctrl = new AbortController();
    apiFetch(
      `/api/players?search=${encodeURIComponent(debouncedMergeQuery)}&limit=10`,
      {
        signal: ctrl.signal,
      },
    )
      .then((res) => (res.ok ? res.json() : []))
      .then((data: PlayerProfile[]) => {
        const filtered = (data ?? []).filter((p) => p.id !== selected.id);
        setMergeResults(filtered);
      })
      .catch(() => {
        /* aborted or network error */
      });
    return () => ctrl.abort();
  }, [debouncedMergeQuery, selected]);

  // Hide stored merge results when no player is selected or the merge
  // query is too short — fresh fetches overwrite when both are valid.
  const displayMergeResults =
    selected && debouncedMergeQuery.trim().length >= 2 ? mergeResults : [];

  const handleMerge = async (mergePlayerId: number) => {
    if (!selected) return;
    if (
      !confirm(
        "Are you sure you want to merge this player? This will move all their GUIDs and stats to the selected player.",
      )
    ) {
      return;
    }

    setMerging(true);
    setError("");
    try {
      const res = await apiFetch(`/api/admin/players/${selected.id}/merge`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ merge_player_id: mergePlayerId }),
      });
      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || "Merge failed");
      }
      setMergeQuery("");
      setMergeResults([]);
      fetchGuids(selected.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Merge failed");
    } finally {
      setMerging(false);
    }
  };

  const handleSplit = async (guidId: number) => {
    if (!confirm("Split this GUID into a separate player?")) return;

    setSplitting(guidId);
    setError("");
    try {
      const res = await apiFetch(`/api/admin/guids/${guidId}/split`, {
        method: "POST",
      });
      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || "Split failed");
      }
      if (selected) fetchGuids(selected.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Split failed");
    } finally {
      setSplitting(null);
    }
  };

  // Right-pane priority mirrors PlayersPage: a fresh search hides the
  // selected detail panel so admins can pivot to a different player
  // without losing context. Gates on `displaySearchResults` so a stale
  // in-flight fetch resolving after a short-query gap can't reappear.
  const showResults = displaySearchResults.length > 0;
  const showBrowse = !showResults && !selected;
  const hasMore = list !== null && list.length < total;

  return (
    <div className="admin-players">
      <div className="admin-section-header">
        <h2>Player administration</h2>
      </div>

      <input
        type="text"
        className="admin-input"
        placeholder="Search players by name or GUID…"
        value={searchQuery}
        onChange={(e) => setSearchQuery(e.target.value)}
      />

      {showResults && (
        <div
          className="player-cards-grid"
          style={{ marginTop: "var(--space-3)" }}
        >
          {displaySearchResults.map((p) => (
            <PlayerCardButton key={p.id} player={p} onSelect={selectPlayer} />
          ))}
        </div>
      )}

      {error && (
        <div className="error-message" style={{ marginTop: "var(--space-3)" }}>
          {error}
        </div>
      )}

      {showBrowse && (
        <>
          <div className="admin-players__list-controls">
            <div className="admin-preset-chips">
              {SORTS.map((s) => (
                <button
                  key={s.key}
                  type="button"
                  className={`admin-preset-chip ${sort === s.key ? "active" : ""}`}
                  onClick={() => changeSort(s.key)}
                >
                  {s.label}
                </button>
              ))}
              <button
                type="button"
                className="admin-preset-chip"
                onClick={() => setDir((d) => (d === "desc" ? "asc" : "desc"))}
                title={
                  dir === "desc"
                    ? "Descending — click for ascending"
                    : "Ascending — click for descending"
                }
              >
                {dir === "desc" ? "↓" : "↑"}
              </button>
            </div>
            <label className="toggle-filter">
              <input
                type="checkbox"
                checked={includeBots}
                onChange={(e) => setIncludeBots(e.target.checked)}
              />
              <span className="toggle-filter__switch" aria-hidden />
              <span className="toggle-filter__label">Include bots</span>
            </label>
          </div>

          {list === null ? (
            <div className="admin-loading">Loading players…</div>
          ) : list.length === 0 ? (
            <div className="admin-empty">No players found</div>
          ) : (
            <>
              <div className="player-cards-grid">
                {list.map((p) => (
                  <PlayerCardButton
                    key={p.id}
                    player={p}
                    onSelect={selectPlayer}
                  />
                ))}
              </div>
              <div className="admin-pagination">
                <span>
                  Showing {list.length} of {total} players
                </span>
                {hasMore && (
                  <button
                    type="button"
                    className="admin-btn"
                    onClick={loadMore}
                    disabled={loadingMore}
                  >
                    {loadingMore ? "Loading…" : "Load more"}
                  </button>
                )}
              </div>
            </>
          )}
        </>
      )}

      {!showResults && selected && (
        <div className="admin-player-detail">
          <button
            type="button"
            className="admin-btn admin-player-detail__back"
            onClick={clearSelection}
          >
            ← All players
          </button>
          <h3>
            <PlayerPortrait model={selected.model} size="lg" />
            <Link to={`/players/${selected.id}`}>
              <ColoredText
                text={
                  selected.is_vr ? stripVRPrefix(selected.name) : selected.name
                }
              />
            </Link>
          </h3>

          <details className="filter-section" open>
            <summary className="filter-section__header">
              <span className="filter-section__caret" aria-hidden="true">
                ▸
              </span>
              <span className="filter-section__title">
                Linked GUIDs
                <span className="admin-section-header__count">
                  {" "}
                  ({guids.length})
                </span>
              </span>
            </summary>
            <div className="filter-section__body">
              {loadingGuids ? (
                <div className="admin-loading">Loading GUIDs…</div>
              ) : (
                <div className="guids-list">
                  {guids.map((guid) => (
                    <div key={guid.id} className="guid-item">
                      <div className="guid-info">
                        <ColoredText text={guid.name} />
                        <span className="guid-hash">{guid.guid}</span>
                        <span className="guid-dates">
                          {formatDate(guid.first_seen)} –{" "}
                          {formatDate(guid.last_seen)}
                        </span>
                      </div>
                      {guids.length > 1 && (
                        <button
                          className="admin-btn-danger"
                          onClick={() => handleSplit(guid.id)}
                          disabled={splitting === guid.id}
                        >
                          {splitting === guid.id ? "Splitting…" : "Split"}
                        </button>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          </details>

          <details className="filter-section" open>
            <summary className="filter-section__header">
              <span className="filter-section__caret" aria-hidden="true">
                ▸
              </span>
              <span className="filter-section__title">
                Merge another player into this one
              </span>
            </summary>
            <div className="filter-section__body">
              <div className="merge-search-input">
                <input
                  type="text"
                  placeholder="Search players by name or GUID…"
                  value={mergeQuery}
                  onChange={(e) => setMergeQuery(e.target.value)}
                />
              </div>
              {displayMergeResults.length > 0 && (
                <div className="merge-results">
                  {displayMergeResults.map((p) => (
                    <div key={p.id} className="merge-result-item">
                      <div className="merge-player-info">
                        <ColoredText
                          text={p.is_vr ? stripVRPrefix(p.name) : p.name}
                        />
                        <span className="merge-player-date">
                          Last seen: {formatDate(p.last_seen)}
                        </span>
                      </div>
                      <button
                        className="admin-btn-danger"
                        onClick={() => handleMerge(p.id)}
                        disabled={merging}
                      >
                        {merging ? "Merging…" : "Merge"}
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </details>
        </div>
      )}
    </div>
  );
}
