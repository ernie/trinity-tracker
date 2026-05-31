// External-link glyph (box with arrow leaving the upper-right
// corner) — pair with target="_blank" links so the new-tab behavior
// is visually telegraphed before the click. Matches the stroke
// weight of DownloadIcon / PlayIcon so the action vocabulary reads
// as one set.
export function ExternalLinkIcon({
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
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinejoin="round"
      strokeLinecap="round"
      className={className}
      aria-hidden="true"
    >
      <path d="M12 9v3a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1h3" />
      <path d="M10 2h4v4" />
      <path d="M7 9 14 2" />
    </svg>
  );
}
