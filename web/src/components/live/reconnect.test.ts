import { describe, expect, test } from "bun:test";
import { initialLive, reduce } from "./reconnect";

describe("reconnect state machine", () => {
  test("starts live", () => {
    expect(initialLive(1000).phase).toBe("live");
  });

  test("stream end → probing", () => {
    const s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    expect(s.phase).toBe("probing");
    expect(s.action).toEqual({ kind: "probe" });
  });

  test("probe 503 → waiting, schedules next probe", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 503, now: 100 });
    expect(s.phase).toBe("waiting");
    expect(s.action).toEqual({ kind: "scheduleProbe", delayMs: 4000 });
  });

  test("gapConfirmed: false while first probe in flight, true after a non-200", () => {
    // streamEnded → probing: cause unknown yet (could be a dropped connection on
    // a still-live stream), so the player shows "Reconnecting…", not a gap.
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    expect(s.gapConfirmed).toBe(false);
    // A non-200 answer confirms a genuine inter-match gap.
    s = reduce(s, { type: "probeResult", status: 503, now: 100 });
    expect(s.gapConfirmed).toBe(true);
  });

  test("gapConfirmed stays false when probes fail outright (airplane mode — status 0)", () => {
    // A failed fetch (status 0) means the relay is unreachable: the viewer's own
    // network is down, NOT an inter-match gap. Must keep showing "Reconnecting…".
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 0, now: 100 });
    expect(s.phase).toBe("waiting"); // still retries
    expect(s.gapConfirmed).toBe(false); // but not a confirmed gap
    s = reduce(s, { type: "probeDue", now: 4100 });
    s = reduce(s, { type: "probeResult", status: 0, now: 4200 });
    expect(s.gapConfirmed).toBe(false); // repeated failures don't promote it
  });

  test("gapConfirmed is sticky: a real gap survives a later transient status-0 probe", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 503, now: 100 }); // real gap → true
    expect(s.gapConfirmed).toBe(true);
    s = reduce(s, { type: "probeDue", now: 4100 });
    s = reduce(s, { type: "probeResult", status: 0, now: 4200 }); // blip mid-gap
    expect(s.gapConfirmed).toBe(true); // stays confirmed
  });

  test("gapConfirmed stays false across a 200-then-reboot (connection dropped, match still live)", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 200, now: 200 });
    expect(s.phase).toBe("live");
    expect(s.gapConfirmed).toBe(false);
  });

  test("reboot state carries gapConfirmed: false for a drop, true for a real gap", () => {
    // Drop: immediate 200, never saw a gap → the reboot is a connection-health event.
    const drop = reduce(
      reduce(initialLive(0), { type: "streamEnded", now: 0 }),
      { type: "probeResult", status: 200, now: 50 },
    );
    expect(drop.action).toEqual({ kind: "reboot" });
    expect(drop.gapConfirmed).toBe(false);

    // Gap: 503 first (match ended), then 200 on the next match → NOT a drop.
    let gap = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    gap = reduce(gap, { type: "probeResult", status: 503, now: 100 });
    gap = reduce(gap, { type: "probeDue", now: 4100 });
    gap = reduce(gap, { type: "probeResult", status: 200, now: 4200 });
    expect(gap.action).toEqual({ kind: "reboot" });
    expect(gap.gapConfirmed).toBe(true);
  });

  test("a fresh streamEnded resets gapConfirmed for the new reconnect episode", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 503, now: 100 }); // gapConfirmed → true
    s = reduce(s, { type: "probeDue", now: 4100 }); // waiting → probing
    s = reduce(s, { type: "probeResult", status: 200, now: 4200 }); // reboot → live
    s = reduce(s, { type: "streamEnded", now: 4300 }); // new episode
    expect(s.gapConfirmed).toBe(false);
  });

  test("wait elapsed → probing again", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 503, now: 100 });
    s = reduce(s, { type: "probeDue", now: 4100 });
    expect(s.phase).toBe("probing");
    expect(s.action).toEqual({ kind: "probe" });
  });

  test("probe 200 → reboots engine and returns to live", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 200, now: 200 });
    expect(s.phase).toBe("live");
    expect(s.action).toEqual({ kind: "reboot" });
  });

  test("give up after window of 503s → offers VOD", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 503, now: 0 });
    s = reduce(s, { type: "probeDue", now: 61000 });
    s = reduce(s, { type: "probeResult", status: 503, now: 61000 });
    expect(s.phase).toBe("ended");
    expect(s.action).toEqual({ kind: "showVod" });
  });

  test("give up after a window of failed probes (status 0) → connection lost, not VOD", () => {
    // All probes failed: we never reached the relay, so this is the viewer's
    // connection, not a finished match. Terminal "lost" (the player shows a retry).
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 0, now: 0 });
    s = reduce(s, { type: "probeDue", now: 61000 });
    s = reduce(s, { type: "probeResult", status: 0, now: 61000 });
    expect(s.phase).toBe("lost");
    expect(s.action).toBe(null);
  });

  test("a real gap before a network outage still gives up to VOD, not lost", () => {
    // We reached the relay at least once (503 → match ended), so even if later
    // probes fail, the terminal state is the finished-match VOD, not "lost".
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 503, now: 0 }); // gapConfirmed
    s = reduce(s, { type: "probeDue", now: 61000 });
    s = reduce(s, { type: "probeResult", status: 0, now: 61000 }); // outage at give-up
    expect(s.phase).toBe("ended");
    expect(s.action).toEqual({ kind: "showVod" });
  });

  test("network error on probe is treated like 503", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 0, now: 100 });
    expect(s.phase).toBe("waiting");
    expect(s.action).toEqual({ kind: "scheduleProbe", delayMs: 4000 });
  });

  test("nginx 502 during the gap is treated like 503", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 502, now: 100 });
    expect(s.phase).toBe("waiting");
    expect(s.action).toEqual({ kind: "scheduleProbe", delayMs: 4000 });
  });

  test("stale probeResult arriving while waiting does not schedule a second probe", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 503, now: 100 }); // -> waiting
    // An in-flight probe's aborted fetch (or a late second response) rejects
    // into probeResult AFTER we've already moved to waiting. It must be a
    // no-op: scheduling here stacks a concurrent setTimeout, doubling the
    // probe rate every cycle.
    s = reduce(s, { type: "probeResult", status: 0, now: 200 });
    expect(s.phase).toBe("waiting"); // unchanged
    expect(s.action).toBe(null); // must NOT schedule another probe
  });

  test("stale probeResult 200 arriving while waiting does not reboot out of order", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 503, now: 100 }); // -> waiting
    // A stale 200 from a prior probe must not fire a reboot from waiting.
    s = reduce(s, { type: "probeResult", status: 200, now: 200 });
    expect(s.phase).toBe("waiting");
    expect(s.action).toBe(null);
  });

  test("duplicate streamEnded during reconnect is ignored (give-up timer not reset)", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 1000 });
    expect(s.reconnectStartedAt).toBe(1000);
    s = reduce(s, { type: "probeResult", status: 503, now: 1100 }); // -> waiting
    s = reduce(s, { type: "streamEnded", now: 5000 }); // spurious duplicate
    expect(s.phase).toBe("waiting"); // unchanged
    expect(s.reconnectStartedAt).toBe(1000); // timer NOT reset
    expect(s.action).toBe(null);
  });
});
