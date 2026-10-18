// Each abrupt reconnect (the viewer's connection dropped while the stream was
// still live) recovers with a full page reload, so drops are stamped into
// sessionStorage to survive it. Enough drops inside the window turn
// "Reconnecting…" into an unstable-connection warning. The history lasts only
// as long as the tab.

export const UNSTABLE_WINDOW_MS = 60_000;
export const UNSTABLE_THRESHOLD = 3;
const STORAGE_KEY = "tv-conn-drops";

/** Keeps the timestamps inside the window ending at `now`. Future stamps come
 *  from the clock moving back between reloads. */
export function pruneDrops(
  times: number[],
  now: number,
  windowMs: number = UNSTABLE_WINDOW_MS,
): number[] {
  return times.filter((t) => t <= now && now - t < windowMs);
}

export function isUnstable(
  times: number[],
  now: number,
  windowMs: number = UNSTABLE_WINDOW_MS,
  threshold: number = UNSTABLE_THRESHOLD,
): boolean {
  return pruneDrops(times, now, windowMs).length >= threshold;
}

// Without storage (private mode, a denied quota) the warning never shows.
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
    /* storage unavailable */
  }
}

/** Call right before the recovery reload. Pruning on write bounds the store. */
export function recordDrop(now: number): void {
  save(pruneDrops([...load(), now], now));
}

export function connectionUnstable(now: number): boolean {
  return isUnstable(load(), now);
}
