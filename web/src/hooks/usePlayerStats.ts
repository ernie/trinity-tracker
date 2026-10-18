import { useState, useEffect, useCallback } from "react";
import { apiFetch } from "../authFetch";
import type { PlayerStatsResponse, TimePeriod } from "../types";

interface UsePlayerStatsResult {
  stats: PlayerStatsResponse | null;
  loading: boolean;
  error: string | null;
  refetch: () => void;
  /** Shows the new featured honor at once and PATCHes the server, reverting
   *  if the request fails. Needs a logged-in session. */
  setFeaturedHonor: (key: string) => Promise<void>;
}

export function usePlayerStats(
  playerId: number | undefined,
  period: TimePeriod,
): UsePlayerStatsResult {
  const [stats, setStats] = useState<PlayerStatsResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [refetchKey, setRefetchKey] = useState(0);

  const refetch = useCallback(() => {
    setRefetchKey((k) => k + 1);
  }, []);

  // Cleared on a player change so the last player's stats never flash. A
  // period change keeps `stats`, so the hero and panel stay mounted. Set
  // during render to skip an extra commit.
  const [prevPlayerId, setPrevPlayerId] = useState(playerId);
  if (playerId !== prevPlayerId) {
    setPrevPlayerId(playerId);
    setStats(null);
    setError(null);
  }

  useEffect(() => {
    if (!playerId) return;

    const ctrl = new AbortController();
    fetch(`/api/players/${playerId}/stats?period=${period}`, {
      signal: ctrl.signal,
    })
      .then((res) => {
        if (!res.ok) throw new Error("Couldn't load this player's stats.");
        return res.json();
      })
      .then((data) => {
        setStats(data);
        setError(null);
      })
      .catch((err) => {
        if (ctrl.signal.aborted) return;
        setError(err.message);
      });
    return () => ctrl.abort();
  }, [playerId, period, refetchKey]);

  const setFeaturedHonor = useCallback(async (key: string) => {
    // priorKey is read inside the updater so a quick second click sees the
    // value the first one applied.
    let priorKey: string | undefined;
    setStats((prev) => {
      if (!prev) return prev;
      priorKey = prev.player.featured_honor;
      return { ...prev, player: { ...prev.player, featured_honor: key } };
    });
    try {
      const res = await apiFetch("/api/account/featured-honor", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ key }),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
    } catch {
      setStats((prev) =>
        prev
          ? { ...prev, player: { ...prev.player, featured_honor: priorKey } }
          : prev,
      );
    }
  }, []);

  return { stats, loading: !stats && !error, error, refetch, setFeaturedHonor };
}
