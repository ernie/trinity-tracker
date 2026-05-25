// Harvester per-player skull-carrier indicator. Mirrors FlagIcon but
// for the team-colored skull silhouettes that Trinity's `flags` extractor
// pulls from the missionpack icons/skull_{red,blue}.tga. Used next to
// player names in live cards to
// signal "this player is currently carrying enemy skulls" — the
// analog of the flag-carry indicator for CTF.
interface SkullIconProps {
  team: 'red' | 'blue'
  size?: 'sm' | 'md' | 'lg'
  className?: string
  title?: string
}

export function SkullIcon({ team, size = 'sm', className = '', title }: SkullIconProps) {
  return (
    <span className={`flag-icon flag-icon-${size} ${className}`} title={title}>
      <img src={`/assets/icons/skull_${team}.png`} alt={`${team} skull`} />
    </span>
  )
}
