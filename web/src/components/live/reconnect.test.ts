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
