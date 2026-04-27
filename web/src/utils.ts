// Format a number with commas (e.g., 1234567 -> "1,234,567")
export function formatNumber(n: number): string {
  return n.toLocaleString();
}

// Strip [VR] prefix from player name when VR badge is shown
// Handles color codes around [VR]: e.g., "^7[VR]^1Name" or "[VR] Name"
export function stripVRPrefix(name: string): string {
  return name.replace(/^(\^[0-9])*\[VR\]\s*/i, '');
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
