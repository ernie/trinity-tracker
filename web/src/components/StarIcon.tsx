// Five-point star used by feature-toggle on MatchCard and MatchDetailPage.
// `filled` flips between hollow stroke (inactive) and solid (active);
// callers drive hue via currentColor (text-dim at rest, ember when on).
export function StarIcon({
  size = 12,
  filled = false,
  className,
}: {
  size?: number;
  filled?: boolean;
  className?: string;
}) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 12 12"
      fill={filled ? "currentColor" : "none"}
      stroke="currentColor"
      strokeWidth="1.2"
      strokeLinejoin="round"
      strokeLinecap="round"
      className={className}
      aria-hidden="true"
    >
      <polygon points="6,1 7.47,4.51 11.27,4.84 8.39,7.34 9.27,11.05 6,9.06 2.73,11.05 3.61,7.34 0.73,4.84 4.53,4.51" />
    </svg>
  );
}
