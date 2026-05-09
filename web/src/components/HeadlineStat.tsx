import { formatNumber } from '../utils'

interface HeadlineStatProps {
  label: string
  /** Numbers get tabular-nums + comma formatting; pre-formatted strings
   *  pass through verbatim (e.g. K/D ratios already to 2 decimals). */
  value: number | string
  title?: string
}

// Big-number stat cell used in the player profile hero strip and modal
// hero strip. Number in Barlow 700, label in Cinzel uppercase tracked.
// CSS sizes (.player-profile vs .player-stats-modal) drive the pixel scale.
export function HeadlineStat({ label, value, title }: HeadlineStatProps) {
  const display = typeof value === 'number' ? formatNumber(value) : value
  return (
    <div className="headline-stat" title={title}>
      <div className="headline-stat__value">{display}</div>
      <div className="headline-stat__label">{label}</div>
    </div>
  )
}
