// "Actively attacking an objective" row indicator. Shape matches
// baseq3's menu/art/3_cursor2 cursor (ring + 4 ticks + center dot);
// drawn inline so the team tint is just an attribute, no asset to ship.
interface TargetReticleIconProps {
  team: "red" | "blue";
  size?: "sm" | "md" | "lg";
  className?: string;
  title?: string;
}

const SIZE_PX = { sm: 14, md: 18, lg: 22 } as const;
const TEAM_FILL: Record<TargetReticleIconProps["team"], string> = {
  red: "#ff5050",
  blue: "#5096ff",
};

export function TargetReticleIcon({
  team,
  size = "sm",
  className = "",
  title,
}: TargetReticleIconProps) {
  const px = SIZE_PX[size];
  const fill = TEAM_FILL[team];
  return (
    <span
      className={`reticle-icon reticle-icon-${size} ${className}`}
      title={title}
    >
      <svg
        viewBox="0 0 24 24"
        width={px}
        height={px}
        role={title ? "img" : undefined}
        aria-label={title}
      >
        <g fill="none" stroke={fill} strokeWidth="2" strokeLinecap="butt">
          <circle cx="12" cy="12" r="6.5" />
          <line x1="12" y1="0" x2="12" y2="4" />
          <line x1="12" y1="20" x2="12" y2="24" />
          <line x1="0" y1="12" x2="4" y2="12" />
          <line x1="20" y1="12" x2="24" y2="12" />
        </g>
        <circle cx="12" cy="12" r="1.4" fill={fill} />
      </svg>
    </span>
  );
}
