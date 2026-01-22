interface FlagIconProps {
  team: 'red' | 'blue'
  status?: 'base' | 'taken' | 'dropped'
  size?: 'sm' | 'md' | 'lg'
  className?: string
  title?: string
}

export function FlagIcon({ team, status = 'base', size = 'sm', className = '', title }: FlagIconProps) {
  // Use different icons based on status
  // base = flag silhouette, taken = person running, dropped = question mark
  const iconBase = status === 'base' ? 'flag_in_base'
    : status === 'taken' ? 'flag_capture'
    : 'flag_missing'

  return (
    <span className={`flag-icon flag-icon-${size} ${className}`} title={title}>
      <img src={`/assets/flags/${iconBase}_${team}.png`} alt={`${team} flag`} />
    </span>
  )
}
