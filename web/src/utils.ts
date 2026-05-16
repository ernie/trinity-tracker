// Format a number with commas (e.g., 1234567 -> "1,234,567")
export function formatNumber(n: number): string {
  return n.toLocaleString();
}

// Strip [VR] prefix from player name when VR badge is shown
// Handles color codes around [VR]: e.g., "^7[VR]^1Name" or "[VR] Name"
export function stripVRPrefix(name: string): string {
  return name.replace(/^(\^[0-9])*\[VR\]\s*/i, '');
}

// Canonical display name for a player record. Strips the [VR] prefix
// when the player is VR (the badge already conveys it). Anywhere a
// player's name is rendered alongside its badges, prefer this over
// inlining the ternary.
export function displayPlayerName(player: { name: string; is_vr?: boolean }): string {
  return player.is_vr ? stripVRPrefix(player.name) : player.name;
}

// serverDisplay composes the canonical UI string for a server from
// its (source, key) identity. Single-source installs see just the
// key; multi-source see "<source> / <key>".
export function serverDisplay(source: string | undefined, key: string | undefined, opts?: { hasMultipleSources?: boolean }): string {
  const k = key || '';
  if (!source) return k;
  if (opts && opts.hasMultipleSources === false) return k;
  return `${source} / ${k}`;
}

// Parse a demo-player time URL param into seconds.
// Accepts plain seconds ("90") or short form ("1m30s", "2m", "45s", "1m30").
// Returns null on garbage; consumers treat null as "ignore".
export function parseTimeParam(raw: string): number | null {
  const s = raw.trim().toLowerCase();
  if (!s) return null;
  if (/^\d+$/.test(s)) return Number(s);
  const m = s.match(/^(?:(\d+)m)?(\d+)?s?$/);
  if (!m || (m[1] === undefined && m[2] === undefined)) return null;
  return Number(m[1] ?? 0) * 60 + Number(m[2] ?? 0);
}

// Inverse of parseTimeParam: produce the short form Share emits.
// 90 -> "1m30s", 120 -> "2m", 45 -> "45s".
export function formatTimeParam(secs: number): string {
  const s = Math.max(0, Math.floor(secs));
  const m = Math.floor(s / 60);
  const r = s % 60;
  if (m === 0) return `${r}s`;
  if (r === 0) return `${m}m`;
  return `${m}m${r}s`;
}
