// Small play-triangle glyph used by demo-watch links on the landing
// page, MatchCard demo actions, and anywhere else the prior emoji "▶"
// appeared. Thin-line outline matches the visual weight of the "↓"
// download glyph rendered next to it on MatchCard, so the action pair
// reads as a coherent set rather than two different icon styles.
// currentColor on stroke lets callers drive the hue via CSS (text-dim
// at rest, lifts to ember on hover).
export function PlayIcon({
  size = 11,
  className,
}: {
  size?: number;
  className?: string;
}) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 12 12"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.4"
      strokeLinejoin="round"
      className={className}
      aria-hidden="true"
    >
      <polygon points="3.2,2.2 9.8,6 3.2,9.8" />
    </svg>
  );
}
