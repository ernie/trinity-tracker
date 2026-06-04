// Cross-reload connection-health tracking for the live player.
//
// An abrupt reconnect — our playback buffer drained while the stream was still
// live, i.e. the viewer's connection dropped, not the match ending (see
// reconnect.ts `gapConfirmed`) — recovers via a full window.location.reload(),
// which wipes all in-memory React state. To notice a *pattern* of drops we stamp
// each one into sessionStorage; the timestamps survive the reloads, so after
// enough drops inside a rolling window the player can escalate from a neutral
// "Reconnecting…" to an explicit "your connection looks unstable" warning.
//
// sessionStorage (not localStorage) scopes this to the current tab's viewing
// session — close the tab and the slate is clean; a fresh visit never inherits a
// prior session's bad-wifi history.

export const UNSTABLE_WINDOW_MS = 60_000;
export const UNSTABLE_THRESHOLD = 3;
const STORAGE_KEY = "tv-conn-drops";

// --- pure core (DOM-free, unit-tested in connectionHealth.test.ts) ---

/** Keep only timestamps inside the rolling window ending at `now`. Future stamps
 *  (a backwards clock change between reloads) are dropped as noise. */
export function pruneDrops(
  times: number[],
  now: number,
  windowMs: number = UNSTABLE_WINDOW_MS,
): number[] {
  return times.filter((t) => t <= now && now - t < windowMs);
}

/** A connection reads as "unstable" once `threshold` drops land in the window. */
export function isUnstable(
  times: number[],
  now: number,
  windowMs: number = UNSTABLE_WINDOW_MS,
  threshold: number = UNSTABLE_THRESHOLD,
): boolean {
  return pruneDrops(times, now, windowMs).length >= threshold;
}

// --- sessionStorage shell (no-ops where storage is unavailable, e.g. private
//     mode or a denied quota — the feature degrades to "never escalates") ---

function load(): number[] {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY);
    const parsed: unknown = raw ? JSON.parse(raw) : [];
    return Array.isArray(parsed)
      ? parsed.filter((t): t is number => typeof t === "number")
      : [];
  } catch {
    return [];
  }
}

function save(times: number[]): void {
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(times));
  } catch {
    /* storage unavailable — silently skip; escalation just won't trigger */
  }
}

/** Stamp an abrupt reconnect. Call right before the recovery reload so the drop
 *  outlives it. Prunes on write so the store can't grow without bound. */
export function recordDrop(now: number): void {
  save(pruneDrops([...load(), now], now));
}

/** Has this viewing session dropped often enough to look unstable? */
export function connectionUnstable(now: number): boolean {
  return isUnstable(load(), now);
}
