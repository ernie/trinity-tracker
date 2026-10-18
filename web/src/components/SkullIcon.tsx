// Marks a Harvester player carrying skulls, as FlagIcon marks a CTF carrier.
interface SkullIconProps {
  team: "red" | "blue";
  size?: "sm" | "md" | "lg";
  className?: string;
  title?: string;
}

export function SkullIcon({
  team,
  size = "sm",
  className = "",
  title,
}: SkullIconProps) {
  return (
    <span className={`flag-icon flag-icon-${size} ${className}`} title={title}>
      <img src={`/assets/icons/skull_${team}.png`} alt={`${team} skull`} />
    </span>
  );
}
