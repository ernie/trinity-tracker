import { StatItem } from "./StatItem";
import { StarIcon } from "./StarIcon";
import { HONORS, DEFAULT_FEATURED_HONOR_KEY } from "../constants/honors";
import type { PlayerStatsResponse } from "../types";

interface HonorsPanelProps {
  stats: PlayerStatsResponse["stats"];
  /** The featured honor, which PlayerHero shows beside the portrait.
   *  Defaults to 'victories' until the owner picks one. */
  featuredKey?: string;
  /** Gives each cell a star button that features its honor. */
  isOwner?: boolean;
  /** The caller saves the new key. */
  onFeaturedChange?: (key: string) => void;
}

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
