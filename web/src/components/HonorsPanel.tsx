import { StatItem } from './StatItem'
import { HONORS } from '../constants/honors'
import type { PlayerStatsResponse } from '../types'

interface HonorsPanelProps {
  stats: PlayerStatsResponse['stats']
}

// "Honors" panel — medal-grid driven by the HONORS constant.
export function HonorsPanel({ stats }: HonorsPanelProps) {
  return (
    <section className="player-panel">
      <h4 className="player-panel__heading">Honors</h4>
      <div className="stats-grid">
        {HONORS.map((honor) => (
          <StatItem
            key={honor.label}
            label={honor.label}
            value={honor.value(stats)}
            backgroundIcon={honor.icon}
          />
        ))}
      </div>
    </section>
  )
}
