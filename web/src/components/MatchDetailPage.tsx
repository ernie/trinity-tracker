import { useState, useEffect } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { MatchCard } from "./MatchCard";
import { Breadcrumbs } from "./Breadcrumbs";
import { useAuth } from "../hooks/useAuth";
import { apiFetch } from "../authFetch";
import { useLiveData } from "../contexts/LiveDataContext";
import { formatExitReason } from "./cards/format";
import { StarIcon } from "./StarIcon";
import type { MatchSummary } from "../types";

export function MatchDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [searchParams] = useSearchParams();
  const playerParam = Number(searchParams.get("player"));
  const highlightPlayerId =
    Number.isInteger(playerParam) && playerParam > 0 ? playerParam : undefined;
  const { showPlayer } = useLiveData();
  const { auth } = useAuth();
  const [match, setMatch] = useState<MatchSummary | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [featurePending, setFeaturePending] = useState(false);
  const [refetchToken, setRefetchToken] = useState(0);

  useEffect(() => {
    if (!id) return;
    const ctrl = new AbortController();
    fetch(`/api/matches/${id}`, { signal: ctrl.signal })
      .then(async (res) => {
        if (res.status === 404) {
          setError("Match not found");
          return null;
        }
        if (!res.ok) {
          setError("Couldn't load this match.");
          return null;
        }
        return res.json() as Promise<MatchSummary>;
      })
      .then((data) => {
        if (!data) return;
        setMatch(data);
        setError(null);
      })
      .catch((e) => {
        if (ctrl.signal.aborted) return;
        console.error("Failed to fetch match:", e);
        setError("Couldn't load this match.");
      });
    return () => ctrl.abort();
  }, [id, refetchToken]);

  // A refetch keeps the optimistic match on screen until the server
  // answers.
  const loading = !match && !error;

  const toggleFeatured = async () => {
    if (!match) return;
    const next = !match.is_featured;
    setFeaturePending(true);
    try {
      const res = await apiFetch(`/api/admin/matches/${match.id}/feature`, {
        method: next ? "POST" : "DELETE",
      });
      if (!res.ok) {
        console.error("Feature toggle failed:", res.status);
        return;
      }
      setMatch({ ...match, is_featured: next });
      setRefetchToken((t) => t + 1);
    } finally {
      setFeaturePending(false);
    }
  };

  return (
    <div className="match-detail-page">
      {match && (
        <Breadcrumbs
          crumbs={[
            { label: "Matches", to: "/matches" },
            {
              label: (() => {
                const date = match.started_at
                  ? new Date(match.started_at).toLocaleDateString()
                  : "";
                const map = match.map_name ?? "";
                return [date, map].filter(Boolean).join(" · ") || "Match";
              })(),
            },
          ]}
        />
      )}

      <div className="match-detail-content">
        {loading ? (
          <div className="stats-loading">Loading match…</div>
        ) : error ? (
          <div className="stats-error">{error}</div>
        ) : match ? (
          <>
            {(match.exit_reason || (auth.isAdmin && match.demo_url)) && (
              <div className="match-detail__meta">
                {match.exit_reason ? (
                  <p className="match-detail__exit">
                    Match ended: {formatExitReason(match.exit_reason)}
                  </p>
                ) : (
                  <span />
                )}
                {auth.isAdmin && match.demo_url && (
                  <button
                    type="button"
                    className={`match-feature-toggle${match.is_featured ? " is-featured" : ""}`}
                    onClick={toggleFeatured}
                    disabled={featurePending}
                    title="The landing page's Watch a fight door can play any featured match"
                  >
                    <StarIcon filled={match.is_featured} />
                    {match.is_featured ? "Featured" : "Feature"}
                  </button>
                )}
              </div>
            )}
            <div className="match-detail-card-container">
              <MatchCard
                match={match}
                onPlayerClick={showPlayer}
                highlightPlayerId={highlightPlayerId}
              />
            </div>
          </>
        ) : null}
      </div>
    </div>
  );
}
