// Extra view margin so hair/hoods overflow the portrait box instead of cropping.
export const HEAD_OVERFLOW = 1.4;

// Beyond this many visible heads we drop to 30fps to protect the frame budget.
const FPS_DROP_VISIBLE_COUNT = 4;

export function frameIntervalMs(
  visibleCount: number,
  coarsePointer: boolean,
): number {
  const fps = coarsePointer || visibleCount > FPS_DROP_VISIBLE_COUNT ? 30 : 60;
  return 1000 / fps;
}

// Deterministic per-head idle phase offset (ms) so heads don't glance in lockstep.
export function phaseOffsetFor(index: number): number {
  return index * 137;
}
