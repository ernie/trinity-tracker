import { expect, test } from "bun:test";
import {
  HonkClock,
  HONK_RETURN_MS,
  honkFxCard,
  honkFxFrame,
  honkPose,
  type HonkFx,
} from "./headHonk";

const fx: HonkFx = {
  prefix: "gfx/geese/honk",
  frames: 5,
  msec: 300,
  delay: 0,
  width: 45,
  height: 36,
  pivotX: 0,
  pivotY: 0,
  angle: 0,
};

test("drawing frames step through the burst and end after it", () => {
  expect([0, 59, 60, 299, 300].map((t) => honkFxFrame(fx, t))).toEqual([
    0, 0, 1, 4, -1,
  ]);
  expect(honkFxFrame({ ...fx, delay: 100 }, 99)).toBe(-1);
  expect(honkFxFrame({ ...fx, delay: 100 }, 160)).toBe(1);
});

test("a beak pointing right on screen lays the card along it", () => {
  const c = honkFxCard(fx, [1, 0], 0);
  expect(c.right[0]).toBeCloseTo(1);
  expect(c.right[1]).toBeCloseTo(0);
  expect(c.up[0]).toBeCloseTo(0);
  expect(c.up[1]).toBeCloseTo(1);
});

test("a beak facing the camera eases the card toward vertical", () => {
  const c = honkFxCard(fx, [0, 0], -1);
  expect(c.right[0]).toBeCloseTo(0);
  expect(c.right[1]).toBeCloseTo(-1);
});

test("a beak pointing left mirrors the card so its up stays up", () => {
  const c = honkFxCard(fx, [-1, 0], 0);
  expect(c.up[1]).toBeGreaterThan(0);
});

test("the pose plays the honk frames, then blends back to rest", () => {
  expect(honkPose(0, 11, 20)).toEqual({ a: 1, b: 2, t: 0 });
  const mid = honkPose(75, 11, 20)!;
  expect([mid.a, mid.b]).toEqual([2, 3]);
  expect(mid.t).toBeCloseTo(0.5);
  const back = honkPose(550 + HONK_RETURN_MS / 2, 11, 20)!;
  expect([back.a, back.b]).toEqual([11, 0]);
  expect(back.t).toBeCloseTo(0.5);
  expect(honkPose(550 + HONK_RETURN_MS, 11, 20)).toBeNull();
});

test("a honk in progress refuses another", () => {
  const clock = new HonkClock(700);
  expect(clock.start(1000)).toBe(true);
  expect(clock.start(1200)).toBe(false);
  expect(clock.since(1200)).toBe(200);
  expect(clock.since(1700)).toBeNull();
  expect(clock.start(1700)).toBe(true);
});
