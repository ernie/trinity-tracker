import { useCallback, useEffect, useState } from 'react'
import { useAuth } from './useAuth'
import type { MySources } from '../types'

const EMPTY: MySources = { sources: [], has_pending: false }

// useMySources polls /api/sources/mine for the logged-in user. Returns
// an empty list when not logged in or before the first fetch resolves.
// 30s poll keeps the button label honest after admin approval/rejection
// without spamming.
export function useMySources(): {
  data: MySources
  loading: boolean
  error: string | null
  refresh: () => void
} {
  const { auth } = useAuth()
  const { token, isAuthenticated } = auth
  const [data, setData] = useState<MySources>(EMPTY)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const fetchMine = useCallback(async () => {
    if (!isAuthenticated || !token) {
      setData(EMPTY)
      return
    }
    setLoading(true)
    try {
      const r = await fetch('/api/sources/mine', {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!r.ok) throw new Error(`HTTP ${r.status}`)
      const body = (await r.json()) as MySources
      setData({
        sources: body.sources ?? [],
        has_pending: body.has_pending ?? false,
      })
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'fetch failed')
    } finally {
      setLoading(false)
    }
  }, [token, isAuthenticated])

  useEffect(() => {
    fetchMine()
    if (!isAuthenticated) return
    const id = setInterval(fetchMine, 30_000)
    return () => clearInterval(id)
  }, [fetchMine, isAuthenticated])

  return { data, loading, error, refresh: fetchMine }
}
