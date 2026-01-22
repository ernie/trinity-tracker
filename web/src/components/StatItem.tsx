export interface StatItemProps {
  label: string
  value: number | string
  className?: string
  subscript?: number
  title?: string
}

export function StatItem({ label, value, className, subscript, title }: StatItemProps) {
  return (
    <div className="stat-item" title={title}>
      <div className={`stat-value ${className ?? ''}`}>
        {value}
        {subscript !== undefined && subscript > 0 && <sub>{subscript}</sub>}
      </div>
      <div className="stat-label">{label}</div>
    </div>
  )
}
