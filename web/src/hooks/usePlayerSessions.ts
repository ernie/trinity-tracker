import { useState, useEffect, useCallback } from 'react'
import type { PlayerSession } from '../types'

interface UsePlayerSessionsResult {
  sessions: PlayerSession[]
  loading: boolean
  error: string | null
  hasMore: boolean
  loadMore: () => void
}

export function usePlayerSessions(
  playerId: number | undefined,
  token: string | null
): UsePlayerSessionsResult {
  const [sessions, setSessions] = useState<PlayerSession[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [hasMore, setHasMore] = useState(true)

  const fetchSessions = useCallback(
    async (beforeId?: number) => {
      if (!playerId || !token) {
        setSessions([])
        return
      }

      setLoading(true)
      setError(null)

      try {
        let url = `/api/players/${playerId}/sessions?limit=10`
        if (beforeId) {
          url += `&before=${beforeId}`
        }

        const res = await fetch(url, {
          headers: {
            Authorization: `Bearer ${token}`,
          },
        })

        if (!res.ok) {
          if (res.status === 401) {
            throw new Error('Unauthorized')
          }
          if (res.status === 403) {
            throw new Error('Admin access required')
          }
          throw new Error('Failed to load sessions')
        }

        const data: PlayerSession[] = await res.json()

        if (beforeId) {
          setSessions((prev) => [...prev, ...data])
        } else {
          setSessions(data)
        }

        setHasMore(data.length === 10)
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load sessions')
      } finally {
        setLoading(false)
      }
    },
    [playerId, token]
  )

  useEffect(() => {
    // Reset on player/token change so a new caller doesn't briefly see
    // the previous player's sessions while the next page is in flight.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setSessions([])
    setHasMore(true)
    fetchSessions()
  }, [fetchSessions])

  const loadMore = useCallback(() => {
    if (sessions.length > 0 && hasMore && !loading) {
      const lastSession = sessions[sessions.length - 1]
      fetchSessions(lastSession.id)
    }
  }, [sessions, hasMore, loading, fetchSessions])

  return { sessions, loading, error, hasMore, loadMore }
}
