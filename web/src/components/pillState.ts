import type { ConnectionStatus } from "../useWebSocket";

// Shared by StatusPill and the landing page's hero pill so the two agree.
// OFFLINE means the feed is unreachable; a connection in progress reads as
// quiet, so a cold load or tab return doesn't flash it.
export interface PillState {
  stateClass: "live" | "quiet" | "offline";
  label: string;
}

export function derivePillState(
  status: ConnectionStatus,
  humansOnline: number,
): PillState {
  if (status === "down") {
    return { stateClass: "offline", label: "OFFLINE" };
  }
  if (status === "open" && humansOnline > 0) {
    return { stateClass: "live", label: `${humansOnline} LIVE` };
  }
  return { stateClass: "quiet", label: "STANDING BY" };
}
