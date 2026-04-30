import { useState, useEffect } from 'react'

export interface ReleaseInfo {
  repo: string
  displayName: string
  version: string | null
  url: string
  bundled: boolean
}

const REPOS = [
  { repo: 'trinity', displayName: 'Trinity Mod', bundled: false },
  { repo: 'trinity-engine', displayName: 'Trinity Engine', bundled: true },
  { repo: 'q3vr', displayName: 'Quake 3 VR', bundled: true },
  { repo: 'ioq3quest', displayName: 'Quake3Quest', bundled: true },
]

const CACHE_KEY = 'github-releases'
const CACHE_TTL = 60 * 60 * 1000 // 1 hour

interface CacheEntry {
  ts: number
  releases: ReleaseInfo[]
}

function getCached(): ReleaseInfo[] | null {
  try {
    const raw = sessionStorage.getItem(CACHE_KEY)
    if (!raw) return null
    const entry: CacheEntry = JSON.parse(raw)
    if (Date.now() - entry.ts > CACHE_TTL) return null
    return entry.releases
  } catch {
    return null
  }
}

function setCache(releases: ReleaseInfo[]) {
  try {
    const entry: CacheEntry = { ts: Date.now(), releases }
    sessionStorage.setItem(CACHE_KEY, JSON.stringify(entry))
  } catch {
    // sessionStorage full or unavailable
  }
}

export function useGitHubReleases() {
  const [releases, setReleases] = useState<ReleaseInfo[]>(() =>
    getCached() ?? REPOS.map(r => ({
      repo: r.repo,
      displayName: r.displayName,
      version: null,
      url: `https://github.com/ernie/${r.repo}/releases`,
      bundled: r.bundled,
    }))
  )
  const [loading, setLoading] = useState(() => getCached() === null)

  useEffect(() => {
    // Cache hit was applied during state init — nothing to fetch.
    if (getCached()) return

    const promises = REPOS.map(r =>
      fetch(`https://api.github.com/repos/ernie/${r.repo}/releases/latest`)
        .then(res => {
          if (!res.ok) throw new Error(`${res.status}`)
          return res.json()
        })
        .then(data => ({
          repo: r.repo,
          displayName: r.displayName,
          version: data.tag_name as string,
          url: `https://github.com/ernie/${r.repo}/releases/latest`,
          bundled: r.bundled,
        }))
        .catch(() => ({
          repo: r.repo,
          displayName: r.displayName,
          version: null,
          url: `https://github.com/ernie/${r.repo}/releases`,
          bundled: r.bundled,
        }))
    )

    Promise.all(promises).then(results => {
      setReleases(results)
      setCache(results)
      setLoading(false)
    })
  }, [])

  return { releases, loading }
}
