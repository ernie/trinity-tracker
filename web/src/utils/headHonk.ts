// Ports of the game's head-effect math (cg_headanim.c) for the tracker's honk.

export interface HonkFx {
  prefix: string;
  frames: number;
  msec: number;
  delay: number;
  width: number;
  height: number;
  pivotX: number;
  pivotY: number;
  angle: number;
}

export interface HonkCard {
  right: [number, number];
  up: [number, number];
  center: [number, number];
}

// At portrait size, this share of the configured size matches the game's look.
export const HONK_FX_SCALE = 0.35;
export const HONK_RETURN_MS = 150;
const FX_BLEND = 0.5;

export function honkFxFrame(fx: HonkFx, msecSince: number): number {
  const age = msecSince - fx.delay;
  if (age < 0 || age >= fx.msec) return -1;
  return Math.floor((age * fx.frames) / fx.msec);
}

export function honkFxCard(
  fx: HonkFx,
  beakOnScreen: [number, number],
  beakDepth: number,
): HonkCard {
  let length = Math.hypot(beakOnScreen[0], beakOnScreen[1]);
  const a = (fx.angle * Math.PI) / 180;
  const c = Math.cos(a);
  const s = Math.sin(a);
  const along = beakDepth > 0 ? 1 : -1;
  // A beak near the view line has no reliable screen direction, so the card
  // eases toward vertical.
  const w = Math.min(1, length / FX_BLEND);
  const d: [number, number] = [
    length > 1e-6 ? (w * beakOnScreen[0]) / length : 0,
    (length > 1e-6 ? (w * beakOnScreen[1]) / length : 0) + (1 - w) * along,
  ];
  length = Math.hypot(d[0], d[1]);
  if (length < 1e-3) {
    d[0] = 0;
    d[1] = along;
  } else {
    d[0] /= length;
    d[1] /= length;
  }
  // A beak pointing left mirrors the card, so the drawing's up stays up.
  const u: [number, number] = d[0] >= 0 ? [-d[1], d[0]] : [d[1], -d[0]];
  const right: [number, number] = [c * d[0] + s * u[0], c * d[1] + s * u[1]];
  const up: [number, number] = [c * u[0] - s * d[0], c * u[1] - s * d[1]];
  return {
    right,
    up,
    center: [
      -(fx.pivotX * fx.width * right[0] + fx.pivotY * fx.height * up[0]),
      -(fx.pivotX * fx.width * right[1] + fx.pivotY * fx.height * up[1]),
    ],
  };
}

// Frame indices count the rest pose as 0 and the honk frames from 1.
export function honkPose(
  msecSince: number,
  frames: number,
  fps: number,
): { a: number; b: number; t: number } | null {
  const clipMs = (frames * 1000) / fps;
  if (msecSince < clipMs) {
    const f = (msecSince * fps) / 1000;
    const i = Math.floor(f);
    return { a: 1 + i, b: 1 + Math.min(i + 1, frames - 1), t: f - i };
  }
  const back = (msecSince - clipMs) / HONK_RETURN_MS;
  return back >= 1 ? null : { a: frames, b: 0, t: back };
}

export class HonkClock {
  private startedAt: number | null = null;

  constructor(private readonly durationMs: number) {}

  start(now: number): boolean {
    if (this.since(now) !== null) return false;
    this.startedAt = now;
    return true;
  }

  since(now: number): number | null {
    if (this.startedAt === null) return null;
    const t = now - this.startedAt;
    return t < this.durationMs ? t : null;
  }
}
