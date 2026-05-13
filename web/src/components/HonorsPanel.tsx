import { StatItem } from './StatItem'
import { HONORS } from '../constants/honors'
import type { PlayerStatsResponse } from '../types'

interface HonorsPanelProps {
  stats: PlayerStatsResponse['stats']
}

// "Honors" panel — 3x3 medal grid of the 9 non-featured honors.
// The featured honor (HONORS[0] / Victory by default) is rendered
// alongside the portrait in PlayerHero, so it's omitted here.
// Phase 2 hook: accept a `featuredKey` prop and filter by that instead.
export function HonorsPanel({ stats }: HonorsPanelProps) {
  const [, ...nonFeaturedHonors] = HONORS
  return (
    <section className="player-panel">
      <h4 className="player-panel__heading">Honors</h4>
      <div className="honors-grid">
        {nonFeaturedHonors.map((honor) => (
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
