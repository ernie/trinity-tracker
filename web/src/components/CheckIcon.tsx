// The short strokes need a heavier line than the other 12-grid icons.
export function CheckIcon({
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
      viewBox="0 0 12 12"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinejoin="round"
      strokeLinecap="round"
      className={className}
      aria-hidden="true"
    >
      <polyline points="2.5,6.5 5,9 9.5,3.5" />
    </svg>
  );
}
