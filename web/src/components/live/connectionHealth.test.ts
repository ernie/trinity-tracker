import { describe, expect, test } from "bun:test";
import {
  pruneDrops,
  isUnstable,
  UNSTABLE_WINDOW_MS,
  UNSTABLE_THRESHOLD,
} from "./connectionHealth";

describe("connection-health pruning", () => {
  test("keeps timestamps inside the window, drops older ones", () => {
    const now = 100_000;
    const times = [
      now - (UNSTABLE_WINDOW_MS + 1), // just outside → dropped
      now - UNSTABLE_WINDOW_MS, // exactly window-old → dropped (strict <)
      now - 1, // inside → kept
      now, // now → kept
    ];
    expect(pruneDrops(times, now)).toEqual([now - 1, now]);
  });

  test("discards future timestamps (clock moved backward between reloads)", () => {
    const now = 1000;
    expect(pruneDrops([now + 5000, now - 10], now)).toEqual([now - 10]);
  });
});

describe("connection-health verdict", () => {
  test("below threshold reads as stable", () => {
    const now = 100_000;
    const times = Array.from(
      { length: UNSTABLE_THRESHOLD - 1 },
      (_, i) => now - i * 1000,
    );
    expect(isUnstable(times, now)).toBe(false);
  });

  test("at threshold within the window reads as unstable", () => {
    const now = 100_000;
    const times = Array.from(
      { length: UNSTABLE_THRESHOLD },
      (_, i) => now - i * 1000,
    );
    expect(isUnstable(times, now)).toBe(true);
  });

  test("enough drops but spread beyond the window stays stable", () => {
    const now = 100_000;
    // THRESHOLD drops, but each a full window apart → only one ever co-resident.
    const times = Array.from(
      { length: UNSTABLE_THRESHOLD },
      (_, i) => now - i * UNSTABLE_WINDOW_MS,
    );
    expect(isUnstable(times, now)).toBe(false);
  });
});
