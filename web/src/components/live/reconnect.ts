// Pure cross-match reconnect/map-swap state machine for the live player. The
// impure shell (LivePlayerPage) performs the actions and feeds results back as
// events. Time is injected via `now` (ms) so transitions are deterministic and testable.

export const PROBE_INTERVAL_MS = 4000;
export const GIVE_UP_MS = 60000;

export type Phase = "live" | "probing" | "waiting" | "ended" | "lost";

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
  /** True once a probe gets a real HTTP answer from the relay (any status != 0,
   *  != 200) — the relay is reachable but between sessions, a genuine inter-match
   *  gap. Stays false while the reconnect is "our buffer drained but the stream is
   *  still live" (a dropped/slow connection) OR when probes fail outright (status 0
   *  — the viewer's network is down, e.g. airplane mode). The player uses it to say
   *  "Waiting for the next match" vs "Reconnecting" rather than claiming a map change. */
  gapConfirmed: boolean;
  action: LiveAction;
}

export type LiveEvent =
  | { type: "streamEnded"; now: number }
  | { type: "probeResult"; status: number; now: number }
  | { type: "probeDue"; now: number };

export function initialLive(now: number): LiveState {
  return {
    phase: "live",
    reconnectStartedAt: now,
    gapConfirmed: false,
    action: null,
  };
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
        gapConfirmed: false,
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
        // Next match is live — reboot the engine onto it; back to live. Carry
        // gapConfirmed forward so the reboot side-effect can distinguish a clean
        // inter-match advance (we saw a non-200 gap first) from an abrupt drop
        // (immediate 200 — the stream never actually ended, our connection did),
        // and count only the latter toward connection-health escalation.
        return {
          phase: "live",
          reconnectStartedAt: event.now,
          gapConfirmed: state.gapConfirmed,
          action: { kind: "reboot" },
        };
      }
      // Non-200 → keep retrying until the give-up window elapses, then stop. How we
      // stop depends on WHY we never recovered: if we ever reached the relay
      // (gapConfirmed — it answered 503/502, the match genuinely ended and the next
      // never came), fall back to VOD. If we never got a real answer (all status 0),
      // the viewer's own connection is down — say "connection lost", don't claim the
      // match finished.
      if (event.now - state.reconnectStartedAt >= GIVE_UP_MS) {
        return state.gapConfirmed
          ? { ...state, phase: "ended", action: { kind: "showVod" } }
          : { ...state, phase: "lost", action: null };
      }
      return {
        ...state,
        phase: "waiting",
        // A *real HTTP answer* from the relay (status != 0, e.g. 503/502) means it's
        // reachable but between sessions — a genuine inter-match gap. A status of 0
        // is a failed fetch: we couldn't reach the relay at all, so the viewer's own
        // network is down (airplane mode) — NOT a gap; keep it "Reconnecting…". Sticky
        // once confirmed so a transient blip mid-gap doesn't flip the message back.
        gapConfirmed: state.gapConfirmed || event.status !== 0,
        action: { kind: "scheduleProbe", delayMs: PROBE_INTERVAL_MS },
      };
    }
  }
}
