import { useEffect, useState } from 'react'

export interface SourceInfo {
  source: string
  active: boolean
}

let cached: SourceInfo[] | null = null
let cachedAt = 0
const CACHE_MS = 60_000

// useSources fetches /api/sources and caches the result for a minute.
// Used by the SourceFilter dropdown (inactive sources rendered with
// an "(inactive)" suffix so historical matches can still be filtered)
// and by serverDisplay's auto-suppress on single-source installs.
export function useSources(): { sources: SourceInfo[]; hasMultiple: boolean } {
  const [sources, setSources] = useState<SourceInfo[]>(() =>
    cached && Date.now() - cachedAt < CACHE_MS ? cached : []
  )

  useEffect(() => {
    // Fresh cache was applied during state init — nothing to fetch.
    if (cached && Date.now() - cachedAt < CACHE_MS) return
    let cancelled = false
    fetch('/api/sources')
      .then(r => (r.ok ? r.json() : []))
      .then((list: SourceInfo[]) => {
        if (cancelled) return
        cached = list || []
        cachedAt = Date.now()
        setSources(cached)
      })
      .catch(() => {
        // silent — empty list just means no tags get rendered
      })
    return () => {
      cancelled = true
    }
  }, [])

  return { sources, hasMultiple: sources.length > 1 }
}
