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
