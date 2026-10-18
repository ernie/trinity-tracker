// Shared by Admin → Sources and the My servers drawer so both agree on a
// source's health: a heartbeat under 90s old is healthy, under 10 minutes is
// stale, and anything older is gone.
const STALE_THRESHOLD_MS = 90 * 1000;
const GONE_THRESHOLD_MS = 10 * 60 * 1000;

export type Health = "green" | "stale" | "gone" | "unknown";

export function heartbeatHealth(lastHeartbeatAt?: string): Health {
  if (!lastHeartbeatAt) return "unknown";
  const ts = Date.parse(lastHeartbeatAt);
  if (!Number.isFinite(ts)) return "unknown";
  const age = Date.now() - ts;
  if (age < STALE_THRESHOLD_MS) return "green";
  if (age < GONE_THRESHOLD_MS) return "stale";
  return "gone";
}

export function healthLabel(h: Health): string {
  switch (h) {
    case "green":
      return "healthy";
    case "stale":
      return "stale";
    case "gone":
      return "gone";
    default:
      return "unknown";
  }
}

// timeAgo formats a heartbeat timestamp as "12s ago", "3m ago" or "2h ago".
export function timeAgo(iso?: string): string {
  if (!iso) return "";
  const ms = Date.now() - Date.parse(iso);
  if (!Number.isFinite(ms) || ms < 0) return "";
  if (ms < 60_000) return `${Math.round(ms / 1000)}s ago`;
  if (ms < 3_600_000) return `${Math.round(ms / 60_000)}m ago`;
  return `${Math.round(ms / 3_600_000)}h ago`;
}
