import { StatItem } from "./StatItem";
import { StarIcon } from "./StarIcon";
import { HONORS, DEFAULT_FEATURED_HONOR_KEY } from "../constants/honors";
import type { PlayerStatsResponse } from "../types";

interface HonorsPanelProps {
  stats: PlayerStatsResponse["stats"];
  /** Key of the currently-featured honor (rendered in PlayerHero, omitted
   *  here). Defaults to 'victories' when the owning user hasn't picked one. */
  featuredKey?: string;
  /** When true, each non-featured cell shows a star button to promote it to
   *  featured. Caller threads this in based on whether the viewer owns the
   *  player (auth.playerId === player.id). */
  isOwner?: boolean;
  /** Called with the new key when the viewer clicks a star. Caller is
   *  responsible for the PATCH + state update (typically via
   *  usePlayerStats().setFeaturedHonor). */
  onFeaturedChange?: (key: string) => void;
}

// "Honors" panel — 3x3 medal grid of the 9 non-featured honors. The honor
// matching `featuredKey` is rendered alongside the portrait in PlayerHero,
// so it's omitted here. When the viewer owns the player, each cell gains a
// small star button that promotes that honor to featured.
export function HonorsPanel({
  stats,
  featuredKey,
  isOwner,
  onFeaturedChange,
}: HonorsPanelProps) {
  const activeKey = featuredKey ?? DEFAULT_FEATURED_HONOR_KEY;
  const nonFeaturedHonors = HONORS.filter((h) => h.key !== activeKey);

  return (
    <section className="player-panel">
      <h4 className="player-panel__heading">Honors</h4>
      <div className="honors-grid">
        {nonFeaturedHonors.map((honor) => (
          <div key={honor.key} className="honor-cell">
            <StatItem
              label={honor.label}
              value={honor.value(stats)}
              backgroundIcon={honor.iconElement ?? honor.icon}
            />
            {isOwner && onFeaturedChange && (
              <button
                type="button"
                className="honor-feature-toggle"
                onClick={() => onFeaturedChange(honor.key)}
                aria-label={`Feature ${honor.label} on profile`}
                title={`Feature ${honor.label} on profile`}
              >
                <StarIcon size={14} />
              </button>
            )}
          </div>
        ))}
      </div>
    </section>
  );
}
