// Tiny helpers used by the landing page's hero pulse. Kept colocated with
// the section components they serve so changes to copy/format don't touch
// shared utility modules.

/** Returns `one` when n === 1, else `many`. Used for hero-pulse pluralization. */
export function plural(n: number, one: string, many: string): string {
  return n === 1 ? one : many;
}

/**
 * Formats total seconds as "Xh YYm of fragging" (≥1h, zero-padded minutes)
 * or "Nm of fragging" (<1h). Returns empty string for under a minute, so the
 * caller can skip rendering a "0m of fragging" line.
 */
export function formatFragTime(totalSeconds: number): string {
  if (totalSeconds < 60) return "";
  const totalMin = Math.floor(totalSeconds / 60);
  const h = Math.floor(totalMin / 60);
  const m = totalMin % 60;
  if (h === 0) return `${m}m of fragging`;
  return `${h}h ${m.toString().padStart(2, "0")}m of fragging`;
}
