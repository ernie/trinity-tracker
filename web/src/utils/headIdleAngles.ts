// Port of CG_DrawStatusBarHead (cgame/cg_draw.c): the HUD head faces the
// camera (yaw 180) and idly glances around, picking a new target every
// 100-2100ms and smoothstep-interpolating toward it.
export class HeadIdleAnimator {
  private startTime = 0;
  private endTime = 0;
  private startYaw = 180;
  private endYaw = 180;
  private startPitch = 0;
  private endPitch = 0;

  constructor(private rng: () => number = Math.random) {}

  sample(timeMs: number): { yaw: number; pitch: number } {
    if (timeMs >= this.endTime) {
      this.startTime = this.endTime <= 0 ? timeMs : this.endTime;
      this.startYaw = this.endYaw;
      this.startPitch = this.endPitch;
      this.endTime = timeMs + 100 + this.rng() * 2000;
      this.endYaw = 180 + 20 * Math.cos(this.crandom() * Math.PI);
      this.endPitch = 5 * Math.cos(this.crandom() * Math.PI);
    }
    if (this.startTime > timeMs) this.startTime = timeMs;
    const span = this.endTime - this.startTime;
    let frac = span > 0 ? (timeMs - this.startTime) / span : 0;
    if (frac < 0) frac = 0;
    if (frac > 1) frac = 1;
    frac = frac * frac * (3 - 2 * frac);
    return {
      yaw: this.startYaw + (this.endYaw - this.startYaw) * frac,
      pitch: this.startPitch + (this.endPitch - this.startPitch) * frac,
    };
  }

  private crandom(): number {
    return 2 * this.rng() - 1;
  }
}
