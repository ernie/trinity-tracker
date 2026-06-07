import type { ConnectionStatus } from "../useWebSocket";

// Shared state derivation for the two live-signal pills (StatusPill on
// app routes, the hero pill on the landing page) so their semantics
// can't drift apart. OFFLINE is reserved for a feed that's genuinely
// unreachable (see ConnectionStatus) — connecting reads as quiet, not
// as an alarming flash on every cold load or tab return.
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
