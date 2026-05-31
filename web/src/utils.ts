// Format a number with commas (e.g., 1234567 -> "1,234,567")
export function formatNumber(n: number): string {
  return n.toLocaleString();
}

// Strip the "[VR]" decoration from a player name, at either the
// start or end. Handles surrounding color codes — "^7[VR] Name" or
// "Name [VR]^7". Mirrors internal/discord/name.go:StripVRTag; keep
// the two regexes in sync.
export function stripVRPrefix(name: string): string {
  return name
    .replace(/^(\^[0-9])*\[VR\]\s*/i, "")
    .replace(/\s*\[VR\](\^[0-9])*$/i, "");
}

// Canonical display name for a player record. Strips the [VR] prefix
// when the player is VR (the badge already conveys it). Anywhere a
// player's name is rendered alongside its badges, prefer this over
// inlining the ternary.
export function displayPlayerName(player: {
  name: string;
  is_vr?: boolean;
}): string {
  return player.is_vr ? stripVRPrefix(player.name) : player.name;
}

// serverDisplay composes the canonical UI string for a server from
// its (source, key) identity. Single-source installs see just the
// key; multi-source see "<source> / <key>".
export function serverDisplay(
  source: string | undefined,
  key: string | undefined,
  opts?: { hasMultipleSources?: boolean },
): string {
  const k = key || "";
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

// Mirrors internal/storage/displayname.go:CleanQ3DisplayName. Strips
// forbidden chars (\, ", ;), black color codes (colorIdx 0 via & 7),
// non-printable, leading whitespace, >2 consecutive spaces, 31-byte limit.
export function cleanQ3DisplayName(raw: string): string {
  const maxBytes = 31;
  const buf: string[] = [];
  let len = 0;
  let colorlessLen = 0;
  let spaces = 0;
  let i = 0;

  while (i < raw.length) {
    const ch = raw.charCodeAt(i);

    if (buf.length === 0 && ch <= 32) {
      i++;
      continue;
    }

    if (raw[i] === "^" && i + 1 < raw.length) {
      const next = raw.charCodeAt(i + 1);
      if (next >= 48 && next <= 57) {
        const colorIdx = (next - 48) & 7;
        if (colorIdx === 0) {
          i += 2;
          continue;
        }
        if (len > maxBytes - 2) break;
        buf.push(raw[i], raw[i + 1]);
        len += 2;
        i += 2;
        continue;
      }
    }

    if (ch < 32 || ch > 126 || ch === 92 || ch === 34 || ch === 59) {
      i++;
      continue;
    }

    if (ch === 32) {
      spaces++;
      if (spaces > 2) {
        i++;
        continue;
      }
    } else {
      spaces = 0;
    }

    if (len > maxBytes - 1) break;

    buf.push(raw[i]);
    colorlessLen++;
    len++;
    i++;
  }

  if (colorlessLen === 0) return "";
  return buf.join("");
}

// Strips [VR] prefix, color codes, and normalizes whitespace for
// stable identity matching across client-reported name variants.
export function canonicalizeDisplayName(raw: string): string {
  let s = stripVRPrefix(raw);
  s = s.replace(/\^[0-9]/g, "");
  s = s.replace(/\s+/g, " ");
  return s.trim().toLowerCase();
}
