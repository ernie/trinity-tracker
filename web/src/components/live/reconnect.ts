// Pure cross-match reconnect/map-swap state machine for the live player. The
// impure shell (LivePlayerPage) performs the actions and feeds results back as
// events. Time is injected via `now` (ms) so transitions are deterministic and testable.

export const PROBE_INTERVAL_MS = 4000;
export const GIVE_UP_MS = 60000;

export type Phase = "live" | "probing" | "waiting" | "ended";

export type LiveAction =
  | { kind: "probe" }
  | { kind: "scheduleProbe"; delayMs: number }
  | { kind: "reboot" }
  | { kind: "showVod" }
  | null;

export interface LiveState {
  phase: Phase;
  /** Wall-clock (ms) when the current reconnect attempt began. */
  reconnectStartedAt: number;
  action: LiveAction;
}

export type LiveEvent =
  | { type: "streamEnded"; now: number }
  | { type: "probeResult"; status: number; now: number }
  | { type: "probeDue"; now: number };

export function initialLive(now: number): LiveState {
  return { phase: "live", reconnectStartedAt: now, action: null };
}

export function reduce(state: LiveState, event: LiveEvent): LiveState {
  switch (event.type) {
    case "streamEnded":
      // Begin a reconnect only from live. A duplicate streamEnded while
      // reconnecting (the hook can re-observe it after a resize) is ignored so
      // it can't reset the give-up timer below.
      if (state.phase !== "live") return { ...state, action: null };
      return {
        phase: "probing",
        reconnectStartedAt: event.now,
        action: { kind: "probe" },
      };

    case "probeDue":
      if (state.phase !== "waiting") return { ...state, action: null };
      return { ...state, phase: "probing", action: { kind: "probe" } };

    case "probeResult": {
      // Only a probe we're actively awaiting may drive a transition. An aborted
      // in-flight fetch rejects into probeResult after we've moved to waiting;
      // without this guard it would stack a concurrent probe (doubling the rate)
      // or fire a stale 200 reboot out of order.
      if (state.phase !== "probing") return { ...state, action: null };
      if (event.status === 200) {
        // Next match is live — reboot the engine onto it; back to live.
        return {
          phase: "live",
          reconnectStartedAt: event.now,
          action: { kind: "reboot" },
        };
      }
      // Anything non-200 → keep waiting until the give-up window elapses, then
      // fall back to VOD. The gap is a clean 503, but a transient nginx 502 or 0
      // (network error) can also appear; all are transient and handled identically.
      if (event.now - state.reconnectStartedAt >= GIVE_UP_MS) {
        return { ...state, phase: "ended", action: { kind: "showVod" } };
      }
      return {
        ...state,
        phase: "waiting",
        action: { kind: "scheduleProbe", delayMs: PROBE_INTERVAL_MS },
      };
    }
  }
}
