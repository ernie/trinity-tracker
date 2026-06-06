import { useState, useEffect, useCallback } from "react";
import { apiFetch } from "../authFetch";
import type { PlayerStatsResponse, TimePeriod } from "../types";

interface UsePlayerStatsResult {
  stats: PlayerStatsResponse | null;
  loading: boolean;
  error: string | null;
  refetch: () => void;
  /** Optimistically update the featured honor and PATCH the server.
   *  Reverts the local state if the request fails. Auth is the session
   *  cookie; callers must be logged in. */
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

  // Reset stats whenever the *player* changes — we shouldn't briefly
  // flash the previous player's data. Period changes keep `stats` in
  // place so swapping pills doesn't unmount the hero/panel structure
  // (the visible flicker users complained about). Adjusting state
  // during render so there's no extra commit cycle.
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
        if (!res.ok) throw new Error("Player not found");
        return res.json();
      })
      .then((data) => {
        setStats(data);
        setError(null); // success clears any prior error
      })
      .catch((err) => {
        if (ctrl.signal.aborted) return;
        setError(err.message);
      });
    return () => ctrl.abort();
  }, [playerId, period, refetchKey]);

  const setFeaturedHonor = useCallback(async (key: string) => {
    // Optimistic local update — capture the prior value so we can revert
    // cleanly if the PATCH fails. Read from the closure-current stats so
    // a rapid second click can still see the just-applied value.
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
      // Revert. If the user toggled again between optimistic update and
      // failure, this puts them back to whatever was on the server — a
      // refetch would be safer but the flash is worse than the rare race.
      setStats((prev) =>
        prev
          ? { ...prev, player: { ...prev.player, featured_honor: priorKey } }
          : prev,
      );
    }
  }, []);

  // Loading is derived: nothing yet AND no error to show. During a
  // period swap, `stats` retains the previous values so the panel
  // stays mounted; the new values just slot in when the fetch lands.
  return { stats, loading: !stats && !error, error, refetch, setFeaturedHonor };
}
