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
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    expect(s.gapConfirmed).toBe(false);
    s = reduce(s, { type: "probeResult", status: 503, now: 100 });
    expect(s.gapConfirmed).toBe(true);
  });

  test("gapConfirmed stays false when probes fail outright (status 0)", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 0, now: 100 });
    expect(s.phase).toBe("waiting");
    expect(s.gapConfirmed).toBe(false);
    s = reduce(s, { type: "probeDue", now: 4100 });
    s = reduce(s, { type: "probeResult", status: 0, now: 4200 });
    expect(s.gapConfirmed).toBe(false);
  });

  test("gapConfirmed is sticky: a real gap survives a later transient status-0 probe", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 503, now: 100 });
    expect(s.gapConfirmed).toBe(true);
    s = reduce(s, { type: "probeDue", now: 4100 });
    s = reduce(s, { type: "probeResult", status: 0, now: 4200 });
    expect(s.gapConfirmed).toBe(true);
  });

  test("gapConfirmed stays false across a 200-then-reboot (connection dropped, match still live)", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 200, now: 200 });
    expect(s.phase).toBe("live");
    expect(s.gapConfirmed).toBe(false);
  });

  test("reboot state carries gapConfirmed: false for a drop, true for a real gap", () => {
    const drop = reduce(
      reduce(initialLive(0), { type: "streamEnded", now: 0 }),
      { type: "probeResult", status: 200, now: 50 },
    );
    expect(drop.action).toEqual({ kind: "reboot" });
    expect(drop.gapConfirmed).toBe(false);

    let gap = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    gap = reduce(gap, { type: "probeResult", status: 503, now: 100 });
    gap = reduce(gap, { type: "probeDue", now: 4100 });
    gap = reduce(gap, { type: "probeResult", status: 200, now: 4200 });
    expect(gap.action).toEqual({ kind: "reboot" });
    expect(gap.gapConfirmed).toBe(true);
  });

  test("a fresh streamEnded resets gapConfirmed for the new reconnect episode", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 503, now: 100 });
    s = reduce(s, { type: "probeDue", now: 4100 });
    s = reduce(s, { type: "probeResult", status: 200, now: 4200 });
    s = reduce(s, { type: "streamEnded", now: 4300 });
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
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 0, now: 0 });
    s = reduce(s, { type: "probeDue", now: 61000 });
    s = reduce(s, { type: "probeResult", status: 0, now: 61000 });
    expect(s.phase).toBe("lost");
    expect(s.action).toBe(null);
  });

  test("a real gap before a network outage still gives up to VOD, not lost", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 503, now: 0 });
    s = reduce(s, { type: "probeDue", now: 61000 });
    s = reduce(s, { type: "probeResult", status: 0, now: 61000 });
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
    s = reduce(s, { type: "probeResult", status: 503, now: 100 });
    s = reduce(s, { type: "probeResult", status: 0, now: 200 });
    expect(s.phase).toBe("waiting");
    expect(s.action).toBe(null);
  });

  test("stale probeResult 200 arriving while waiting does not reboot out of order", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 0 });
    s = reduce(s, { type: "probeResult", status: 503, now: 100 });
    s = reduce(s, { type: "probeResult", status: 200, now: 200 });
    expect(s.phase).toBe("waiting");
    expect(s.action).toBe(null);
  });

  test("duplicate streamEnded during reconnect is ignored (give-up timer not reset)", () => {
    let s = reduce(initialLive(0), { type: "streamEnded", now: 1000 });
    expect(s.reconnectStartedAt).toBe(1000);
    s = reduce(s, { type: "probeResult", status: 503, now: 1100 });
    s = reduce(s, { type: "streamEnded", now: 5000 });
    expect(s.phase).toBe("waiting");
    expect(s.reconnectStartedAt).toBe(1000);
    expect(s.action).toBe(null);
  });
});
