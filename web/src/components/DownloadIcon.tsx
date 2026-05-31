// Download glyph (arrow-into-tray) used wherever a download action
// surfaces — the top-nav install link, the demo download button on
// match cards. Stroke weight matches PlayIcon / ArrowIcon so the
// action vocabulary reads as one set.
//
// Note: the 16-unit viewBox + thin-stroke design has more internal
// whitespace than PlayIcon's 12-unit triangle, so when paired with
// PlayIcon at the same `size` the download reads visually smaller.
// Callers in tight icon+label pairings (e.g. MatchCard's demo-action
// row) should pass a slightly larger `size` to compensate.
export function DownloadIcon({
  size = 12,
  className,
}: {
  size?: number;
  className?: string;
}) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinejoin="round"
      strokeLinecap="round"
      className={className}
      aria-hidden="true"
    >
      <path d="M8 2v8M5 7l3 3 3-3M3 12h10" />
    </svg>
  );
}
