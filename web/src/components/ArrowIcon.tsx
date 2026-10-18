// Each direction has its own path so the icon survives ancestor transforms.
type Direction = "right" | "left" | "up" | "down";

const PATHS: Record<Direction, string> = {
  right: "M2 6 H10 M7 3 L10 6 L7 9",
  left: "M10 6 H2 M5 3 L2 6 L5 9",
  up: "M6 10 V2 M3 5 L6 2 L9 5",
  down: "M6 2 V10 M3 7 L6 10 L9 7",
};

export function ArrowIcon({
  size = 12,
  direction = "right",
  className,
}: {
  size?: number;
  direction?: Direction;
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
      strokeLinecap="round"
      className={className}
      aria-hidden="true"
    >
      <path d={PATHS[direction]} />
    </svg>
  );
}
