// Reads smaller than PlayIcon at the same size, so pass a slightly
// larger size when the two sit side by side.
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
