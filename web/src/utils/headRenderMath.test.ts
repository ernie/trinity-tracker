import { expect, test } from "bun:test";
import { frameIntervalMs, phaseOffsetFor } from "./headRenderMath";

test("frameIntervalMs targets 60fps for few heads on a fine pointer", () => {
  expect(frameIntervalMs(3, false)).toBeCloseTo(1000 / 60, 5);
});

test("frameIntervalMs drops to 30fps on a coarse pointer", () => {
  expect(frameIntervalMs(1, true)).toBeCloseTo(1000 / 30, 5);
});

test("frameIntervalMs drops to 30fps when many heads are visible", () => {
  expect(frameIntervalMs(5, false)).toBeCloseTo(1000 / 30, 5);
  expect(frameIntervalMs(4, false)).toBeCloseTo(1000 / 60, 5);
});

test("phaseOffsetFor is deterministic and staggers by index", () => {
  expect(phaseOffsetFor(0)).toBe(0);
  expect(phaseOffsetFor(1)).toBeGreaterThan(0);
  expect(phaseOffsetFor(2)).toBeGreaterThan(phaseOffsetFor(1));
  expect(phaseOffsetFor(3)).toBe(411);
});
