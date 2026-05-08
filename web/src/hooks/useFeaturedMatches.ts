import { useSyncExternalStore } from 'react'

let state: { loaded: boolean; ids: number[] } = { loaded: false, ids: [] }
const listeners = new Set<() => void>()
let fetchStarted = false

function notify() { listeners.forEach((l) => l()) }

function startFetch() {
  if (fetchStarted) return
  fetchStarted = true
  fetch('/api/matches/featured')
    .then((res) => (res.ok ? res.json() : { matches: [] }))
    .catch(() => ({ matches: [] }))
    .then((data: { matches?: number[] }) => {
      state = { loaded: true, ids: data.matches ?? [] }
      notify()
    })
}

function subscribe(cb: () => void): () => void {
  listeners.add(cb)
  startFetch()
  return () => listeners.delete(cb)
}

function getSnapshot() { return state }

/**
 * Returns the current pool of featured match IDs. Empty list while loading
 * (and after a 404). Consumers should treat empty as "fall back to /matches".
 */
export function useFeaturedMatches(): { ids: number[]; loaded: boolean } {
  const snap = useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
  return { ids: snap.ids, loaded: snap.loaded }
}

/** Returns a random ID from `ids`, or null when the list is empty. */
export function pickRandomFeatured(ids: number[]): number | null {
  if (ids.length === 0) return null
  return ids[Math.floor(Math.random() * ids.length)]
}
