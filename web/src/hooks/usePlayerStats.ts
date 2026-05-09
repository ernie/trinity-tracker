import { useState, useEffect, useCallback } from 'react'
import type { PlayerStatsResponse, TimePeriod } from '../types'

interface UsePlayerStatsResult {
  stats: PlayerStatsResponse | null
  loading: boolean
  error: string | null
  refetch: () => void
}

export function usePlayerStats(playerId: number | undefined, period: TimePeriod): UsePlayerStatsResult {
  const [stats, setStats] = useState<PlayerStatsResponse | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [refetchKey, setRefetchKey] = useState(0)

  const refetch = useCallback(() => {
    setRefetchKey(k => k + 1)
  }, [])

  // Reset stats whenever the *player* changes — we shouldn't briefly
  // flash the previous player's data. Period changes keep `stats` in
  // place so swapping pills doesn't unmount the hero/panel structure
  // (the visible flicker users complained about). Adjusting state
  // during render so there's no extra commit cycle.
  const [prevPlayerId, setPrevPlayerId] = useState(playerId)
  if (playerId !== prevPlayerId) {
    setPrevPlayerId(playerId)
    setStats(null)
    setError(null)
  }

  useEffect(() => {
    if (!playerId) return

    const ctrl = new AbortController()
    fetch(`/api/players/${playerId}/stats?period=${period}`, { signal: ctrl.signal })
      .then((res) => {
        if (!res.ok) throw new Error('Player not found')
        return res.json()
      })
      .then((data) => {
        setStats(data)
        setError(null)  // success clears any prior error
      })
      .catch((err) => {
        if (ctrl.signal.aborted) return
        setError(err.message)
      })
    return () => ctrl.abort()
  }, [playerId, period, refetchKey])

  // Loading is derived: nothing yet AND no error to show. During a
  // period swap, `stats` retains the previous values so the panel
  // stays mounted; the new values just slot in when the fetch lands.
  return { stats, loading: !stats && !error, error, refetch }
}
