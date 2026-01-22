import { useState, useEffect } from 'react'
import type { PlayerStatsResponse, TimePeriod } from '../types'

interface UsePlayerStatsResult {
  stats: PlayerStatsResponse | null
  loading: boolean
  error: string | null
}

export function usePlayerStats(playerId: number | undefined, period: TimePeriod): UsePlayerStatsResult {
  const [stats, setStats] = useState<PlayerStatsResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!playerId) {
      setStats(null)
      return
    }

    setLoading(true)
    setError(null)

    fetch(`/api/players/${playerId}/stats?period=${period}`)
      .then((res) => {
        if (!res.ok) throw new Error('Player not found')
        return res.json()
      })
      .then((data) => setStats(data))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false))
  }, [playerId, period])

  return { stats, loading, error }
}
