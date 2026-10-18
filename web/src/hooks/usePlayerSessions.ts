import { useState, useEffect, useCallback } from "react";
import { apiFetch } from "../authFetch";
import type { PlayerSession } from "../types";

interface UsePlayerSessionsResult {
  sessions: PlayerSession[];
  loading: boolean;
  error: string | null;
  hasMore: boolean;
  loadMore: () => void;
}

export function usePlayerSessions(
  playerId: number | undefined,
): UsePlayerSessionsResult {
  const [sessions, setSessions] = useState<PlayerSession[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(true);

  const fetchSessions = useCallback(
    async (beforeId?: number) => {
      if (!playerId) {
        setSessions([]);
        return;
      }

      setLoading(true);
      setError(null);

      try {
        let url = `/api/players/${playerId}/sessions?limit=10`;
        if (beforeId) {
          url += `&before=${beforeId}`;
        }

        const res = await apiFetch(url);

        if (!res.ok) {
          if (res.status === 401) {
            throw new Error("Log in to see sessions.");
          }
          if (res.status === 403) {
            throw new Error("Only admins can see sessions.");
          }
          throw new Error("Couldn't load sessions.");
        }

        // Go's json.Marshal sends a nil slice as null.
        const raw = await res.json();
        const data: PlayerSession[] = Array.isArray(raw) ? raw : [];

        if (beforeId) {
          setSessions((prev) => [...prev, ...data]);
        } else {
          setSessions(data);
        }

        setHasMore(data.length === 10);
      } catch (err) {
        setError(
          err instanceof Error ? err.message : "Couldn't load sessions.",
        );
      } finally {
        setLoading(false);
      }
    },
    [playerId],
  );

  useEffect(() => {
    // Reset so a new player never shows the last one's sessions while loading.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setSessions([]);
    setHasMore(true);
    fetchSessions();
  }, [fetchSessions]);

  const loadMore = useCallback(() => {
    if (sessions.length > 0 && hasMore && !loading) {
      const lastSession = sessions[sessions.length - 1];
      fetchSessions(lastSession.id);
    }
  }, [sessions, hasMore, loading, fetchSessions]);

  return { sessions, loading, error, hasMore, loadMore };
}
