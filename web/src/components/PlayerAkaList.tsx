import { ColoredText } from './ColoredText'
import type { PlayerStatsResponse, PlayerName } from '../types'

interface PlayerAkaListProps {
  /** All recorded names; deduped and the player's primary name removed. */
  names: PlayerStatsResponse['names']
  /** Primary name to filter out — usually `stats.player.name`. */
  primaryName: string
  /** Cap on chips rendered. Modal uses 6, full profile uses 12. */
  max: number
}

// "Also known as" panel — chip cluster of unique alternate names.
// Renders nothing when there's nothing to show; the caller doesn't
// need to guard.
export function PlayerAkaList({ names, primaryName, max }: PlayerAkaListProps) {
  if (!names) return null
  const unique = [...new Set(names.map((n: PlayerName) => n.name))].filter((n) => n !== primaryName)
  if (unique.length === 0) return null

  return (
    <section className="player-panel">
      <h4 className="player-panel__heading">Also known as</h4>
      <div className="aka-list">
        {unique.slice(0, max).map((name, i) => (
          <span key={i} className="aka-chip">
            <ColoredText text={name} />
          </span>
        ))}
      </div>
    </section>
  )
}
