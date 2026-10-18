// Format a number with commas (e.g., 1234567 -> "1,234,567")
export function formatNumber(n: number): string {
  return n.toLocaleString();
}

// Strip a leading or trailing "[VR]" tag and the color codes around it
// ("^7[VR] Name", "Name [VR]^7"). Keep the regexes in step with
// internal/discord/name.go:StripVRTag.
export function stripVRPrefix(name: string): string {
  return name
    .replace(/^(\^[0-9])*\[VR\]\s*/i, "")
    .replace(/\s*\[VR\](\^[0-9])*$/i, "");
}

// A VR player's name drops the [VR] tag, since the badge shows it. Use this
// wherever a name renders beside its badges.
export function displayPlayerName(player: {
  name: string;
  is_vr?: boolean;
}): string {
  return player.is_vr ? stripVRPrefix(player.name) : player.name;
}

// A server's name in the UI: "<source> / <key>", or just the key when
// there's only one source.
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

// Parse a follow-target client number (the engine slot tv_view takes) from a
// URL param. Out-of-range or non-numeric returns null so the receiver silently
// ignores garbage links.
export function parseClientNum(raw: string): number | null {
  if (!/^\d+$/.test(raw)) return null;
  const n = Number(raw);
  return n >= 0 && n < 64 ? n : null;
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

// Mirrors internal/storage/displayname.go:CleanQ3DisplayName: drops \, " and
// ;, black color codes (index 0 after & 7), non-printables, leading
// whitespace and runs of more than 2 spaces, and stops at 31 bytes.
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

// Lowercased, with the [VR] tag, color codes and extra whitespace removed, so
// the same name from different clients matches.
export function canonicalizeDisplayName(raw: string): string {
  let s = stripVRPrefix(raw);
  s = s.replace(/\^[0-9]/g, "");
  s = s.replace(/\s+/g, " ");
  return s.trim().toLowerCase();
}
