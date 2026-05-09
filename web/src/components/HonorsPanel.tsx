import { StatItem } from './StatItem'
import { HONORS } from '../constants/honors'
import type { PlayerStatsResponse } from '../types'

interface HonorsPanelProps {
  stats: PlayerStatsResponse['stats']
}

// "Honors" panel — 8 medal cells (Victory + 4 arena + 3 CTF) with the
// rotated medal as a background icon. Definitions come from the HONORS
// constant so adding/reordering a stat is a one-line edit.
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
