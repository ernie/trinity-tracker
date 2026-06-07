import { useEffect, useState } from "react";
import { MatchCard, formatGameType } from "../MatchCard";
import { NavScroller } from "../NavScroller";
import { SourceFilter } from "../SourceFilter";
import { GAME_TYPES, type GameTypeFilter } from "../../constants/labels";
import { useLiveData } from "../../contexts/LiveDataContext";
import { apiFetch } from "../../authFetch";
import type { MatchSummary } from "../../types";

const BROWSE_PAGE_SIZE = 12;
// The featured pool is expected to stay small; 100 is the API's max limit.
const FEATURED_LIMIT = 100;

// Keep the featured grid in the API's ended_at DESC order when a match is
// featured locally. RFC3339 strings compare correctly as strings.
function insertByEndedAt(
  list: MatchSummary[],
  match: MatchSummary,
): MatchSummary[] {
  const next = [...list.filter((m) => m.id !== match.id), match];
  next.sort((a, b) => (b.ended_at ?? "").localeCompare(a.ended_at ?? ""));
  return next;
}

export function AdminFeatured() {
  const { showPlayer } = useLiveData();

  // Currently featured — fetched without the demo requirement so flagged
  // matches whose demo went missing stay visible (the landing door hides
  // them, which is exactly why an admin needs to see them here).
  const [featured, setFeatured] = useState<MatchSummary[] | null>(null);
  const [featuredError, setFeaturedError] = useState(false);

  useEffect(() => {
    const ctrl = new AbortController();
    fetch(
      `/api/matches?featured=true&include_bot_only=true&limit=${FEATURED_LIMIT}`,
      { signal: ctrl.signal },
    )
      .then((res) => {
        if (!res.ok) throw new Error(`featured fetch: ${res.status}`);
        return res.json();
      })
      .then((data: MatchSummary[] | null) => setFeatured(data ?? []))
      .catch(() => {
        if (!ctrl.signal.aborted) setFeaturedError(true);
      });
    return () => ctrl.abort();
  }, []);

  // Browse — recent matches with demos, candidates for featuring.
  const [gameType, setGameType] = useState<GameTypeFilter>("all");
  const [source, setSource] = useState("");
  const [includeBotOnly, setIncludeBotOnly] = useState(false);
  const [browse, setBrowse] = useState<MatchSummary[]>([]);
  const [browseLoading, setBrowseLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [hasMore, setHasMore] = useState(true);

  // Inlined in both the effect and loadMore (mirrors MatchesPage) — a
  // shared closure would have to join the effect's dependency list.
  const browseQuery = (before?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(BROWSE_PAGE_SIZE));
    params.set("has_demo", "true");
    if (gameType !== "all") params.set("game_type", gameType);
    if (source) params.set("source", source);
    if (includeBotOnly) params.set("include_bot_only", "true");
    if (before) params.set("before", String(before));
    return params.toString();
  };

  useEffect(() => {
    const ctrl = new AbortController();
    async function fetchBrowse() {
      try {
        setBrowseLoading(true);
        setBrowse([]);
        setHasMore(true);

        const params = new URLSearchParams();
        params.set("limit", String(BROWSE_PAGE_SIZE));
        params.set("has_demo", "true");
        if (gameType !== "all") params.set("game_type", gameType);
        if (source) params.set("source", source);
        if (includeBotOnly) params.set("include_bot_only", "true");

        const res = await fetch(`/api/matches?${params.toString()}`, {
          signal: ctrl.signal,
        });
        if (res.ok) {
          const data: MatchSummary[] | null = await res.json();
          const matches = data ?? [];
          setBrowse(matches);
          setHasMore(matches.length === BROWSE_PAGE_SIZE);
        }
      } catch (e) {
        if (!ctrl.signal.aborted) console.error("Failed to fetch matches:", e);
      } finally {
        if (!ctrl.signal.aborted) setBrowseLoading(false);
      }
    }
    fetchBrowse();
    return () => ctrl.abort();
  }, [gameType, source, includeBotOnly]);

  const loadMore = async () => {
    if (loadingMore || !hasMore || browse.length === 0) return;
    try {
      setLoadingMore(true);
      const before = browse[browse.length - 1].id;
      const res = await fetch(`/api/matches?${browseQuery(before)}`);
      if (res.ok) {
        const data: MatchSummary[] | null = await res.json();
        const matches = data ?? [];
        setBrowse((prev) => [...prev, ...matches]);
        setHasMore(matches.length === BROWSE_PAGE_SIZE);
      }
    } catch (e) {
      console.error("Failed to fetch more matches:", e);
    } finally {
      setLoadingMore(false);
    }
  };

  // Both grids render the same matches as independent MatchCard instances,
  // so a confirmed toggle in either one patches both lists here.
  const handleFeatureChange = (matchId: number, isFeatured: boolean) => {
    setBrowse((prev) =>
      prev.map((m) =>
        m.id === matchId ? { ...m, is_featured: isFeatured } : m,
      ),
    );
    if (!isFeatured) {
      setFeatured((prev) => prev && prev.filter((m) => m.id !== matchId));
      return;
    }
    const summary = browse.find((m) => m.id === matchId);
    setFeatured((prev) => {
      if (!prev) return prev;
      if (prev.some((m) => m.id === matchId)) {
        return prev.map((m) =>
          m.id === matchId ? { ...m, is_featured: true } : m,
        );
      }
      return summary
        ? insertByEndedAt(prev, { ...summary, is_featured: true })
        : prev;
    });
  };

  // Featured matches without a demo never render the card's own star
  // toggle (it requires demo_url), so they get an explicit remove button.
  const [removingId, setRemovingId] = useState<number | null>(null);
  const unfeature = async (matchId: number) => {
    setRemovingId(matchId);
    try {
      const res = await apiFetch(`/api/admin/matches/${matchId}/feature`, {
        method: "DELETE",
      });
      if (res.ok) handleFeatureChange(matchId, false);
    } finally {
      setRemovingId(null);
    }
  };

  return (
    <div className="admin-featured">
      <div className="admin-section-header">
        <h2>
          Featured matches
          {featured !== null && (
            <span className="admin-section-header__count">
              {" "}
              ({featured.length})
            </span>
          )}
        </h2>
      </div>
      <p className="admin-featured__hint">
        Featured matches surface at random behind the landing page's
        Watch-a-fight door. Visitors are only served matches with a playable
        demo.
      </p>

      {featuredError ? (
        <div className="error-message">Failed to load featured matches</div>
      ) : featured === null ? (
        <div className="admin-loading">Loading featured matches…</div>
      ) : featured.length === 0 ? (
        <div className="admin-empty">
          Nothing featured yet — star a match below to put it on the landing
          page.
        </div>
      ) : (
        <div className="matches-list matches-browser-list">
          {featured.map((m) => (
            <div key={m.id} className="admin-featured__item">
              <MatchCard
                match={m}
                onPlayerClick={showPlayer}
                onFeatureChange={handleFeatureChange}
              />
              {!m.demo_url && (
                <div className="admin-featured__warning">
                  <span>Demo unavailable — hidden from the landing door.</span>
                  <button
                    className="admin-btn-danger"
                    onClick={() => unfeature(m.id)}
                    disabled={removingId === m.id}
                  >
                    {removingId === m.id ? "Removing…" : "Unfeature"}
                  </button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      <div className="admin-section-header admin-featured__browse-header">
        <h2>Add matches</h2>
      </div>
      <p className="admin-featured__hint">
        Recent matches with demos — use the star on a card to feature it.
      </p>

      <div className="admin-featured__filters">
        <div className="game-type-selector">
          <NavScroller scrollClassName="filter-chips__scroll">
            <div className="filter-chips__strip">
              <button
                key="all"
                className={`game-type-btn ${gameType === "all" ? "active" : ""}`}
                onClick={() => setGameType("all")}
              >
                All
              </button>
              {GAME_TYPES.map((gt) => (
                <button
                  key={gt}
                  className={`game-type-btn ${gameType === gt ? "active" : ""}`}
                  onClick={() => setGameType(gt)}
                >
                  {formatGameType(gt)}
                </button>
              ))}
            </div>
          </NavScroller>
        </div>
        <div className="admin-featured__filter-row">
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
        </div>
      </div>

      {browseLoading ? (
        <div className="admin-loading">Loading matches…</div>
      ) : browse.length === 0 ? (
        <div className="admin-empty">No matches found</div>
      ) : (
        <>
          <div className="matches-list matches-browser-list">
            {browse.map((m) => (
              <MatchCard
                key={m.id}
                match={m}
                onPlayerClick={showPlayer}
                onFeatureChange={handleFeatureChange}
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
                {loadingMore ? "Loading..." : "Load More"}
              </button>
            </div>
          )}
        </>
      )}
    </div>
  );
}
