// Reconnect state machine for the live player. LivePlayerPage performs the
// actions and feeds the results back as events; time comes in as `now` (ms).

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
  /** True once a probe gets a non-200 HTTP answer: the relay is up and between
   *  matches. A failed fetch (status 0) means the viewer's own network is down.
   *  Picks "Waiting for the next match" over "Reconnecting". */
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
      // The hook can report streamEnded again after a resize; only the first
      // one starts the give-up timer.
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
      // An aborted fetch can land here after the move to waiting; only the
      // probe being awaited may act.
      if (state.phase !== "probing") return { ...state, action: null };
      if (event.status === 200) {
        // gapConfirmed rides along so the reboot counts only an abrupt drop
        // (an immediate 200) toward connection health.
        return {
          phase: "live",
          reconnectStartedAt: event.now,
          gapConfirmed: state.gapConfirmed,
          action: { kind: "reboot" },
        };
      }
      // At give-up, a relay that answered means the match ended, so show the
      // recording; a relay never reached means the viewer's connection is lost.
      if (event.now - state.reconnectStartedAt >= GIVE_UP_MS) {
        return state.gapConfirmed
          ? { ...state, phase: "ended", action: { kind: "showVod" } }
          : { ...state, phase: "lost", action: null };
      }
      return {
        ...state,
        phase: "waiting",
        // Sticky, so a blip during a real gap keeps the message.
        gapConfirmed: state.gapConfirmed || event.status !== 0,
        action: { kind: "scheduleProbe", delayMs: PROBE_INTERVAL_MS },
      };
    }
  }
}
