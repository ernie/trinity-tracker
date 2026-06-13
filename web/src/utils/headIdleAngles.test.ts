import { expect, test } from "bun:test";
import { HeadIdleAnimator } from "./headIdleAngles";

test("base yaw faces the camera at 180 on the first sample", () => {
  const a = new HeadIdleAnimator(() => 0.5);
  const { yaw, pitch } = a.sample(0);
  expect(yaw).toBeCloseTo(180, 3);
  expect(pitch).toBeCloseTo(0, 3);
});

test("yaw stays within the HUD drift band ±20°", () => {
  const seq = [0.0, 0.9, 0.3, 0.7, 0.1, 0.8];
  let i = 0;
  const a = new HeadIdleAnimator(() => seq[i++ % seq.length]);
  for (let t = 0; t <= 6000; t += 50) {
    const { yaw, pitch } = a.sample(t);
    expect(yaw).toBeGreaterThanOrEqual(160 - 1e-3);
    expect(yaw).toBeLessThanOrEqual(200 + 1e-3);
    expect(Math.abs(pitch)).toBeLessThanOrEqual(5 + 1e-3);
  }
});
