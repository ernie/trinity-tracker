import { BotBadge } from "./BotBadge";
import { ColoredText } from "./ColoredText";
import { ModelHead } from "./ModelHead";
import { PlayerBadge } from "./PlayerBadge";
import { HeadlineStat } from "./HeadlineStat";
import { HONORS, DEFAULT_FEATURED_HONOR_KEY } from "../constants/honors";
import { displayPlayerName } from "../utils";
import { formatDate, formatDuration } from "../utils/formatters";
import type { PlayerStatsResponse } from "../types";

interface PlayerHeroProps {
  player: PlayerStatsResponse["player"];
  stats: PlayerStatsResponse["stats"];
  /** 'modal' is the compact hero; 'page' is the /players/:id hero. */
  variant: "modal" | "page";
  /** Shown while the fetched player record's name is still empty. */
  fallbackName?: string;
  /** Makes the portrait a button that opens the picker. */
  onPortraitClick?: () => void;
}

export function PlayerHero({
  player,
  stats,
  variant,
  fallbackName,
  onPortraitClick,
}: PlayerHeroProps) {
  const heroName = displayPlayerName(player) || fallbackName || "";
  const NameTag = variant === "modal" ? "h3" : "h2";
  // A stored key can name an honor that no longer exists.
  const featuredKey = player.featured_honor ?? DEFAULT_FEATURED_HONOR_KEY;
  const featured = HONORS.find((h) => h.key === featuredKey) ?? HONORS[0];

  return (
    <header className={`player-hero player-hero--${variant}`}>
      <div className="player-hero__top">
        <div className="player-hero__portrait">
          {onPortraitClick ? (
            <button
              className="player-hero__portrait-btn"
              onClick={onPortraitClick}
              title="Change profile icon"
              aria-label="Change profile icon"
            >
              <ModelHead model={player.model} size="lg" />
            </button>
          ) : (
            <ModelHead model={player.model} size="lg" />
          )}
          {player.is_bot ? (
            <BotBadge isBot skill={5} size={variant === "page" ? "lg" : "md"} />
          ) : (
            <PlayerBadge
              isVerified={player.is_verified}
              isAdmin={player.is_admin}
              isVR={player.is_vr}
              size={variant === "page" ? "lg" : "md"}
            />
          )}
        </div>
        <div className="player-hero__featured" title={featured.label}>
          {/* The wrapper takes the medal's sizing so the count chip
              docks at the same height. */}
          {featured.iconElement ? (
            <span className="player-hero__featured-medal">
              {featured.iconElement}
            </span>
          ) : (
            <img
              className="player-hero__featured-medal"
              src={featured.icon}
              alt={featured.label}
            />
          )}
          <span className="player-hero__featured-count">
            {featured.value(stats)}
          </span>
        </div>
      </div>

      <NameTag className="player-hero__name">
        <ColoredText text={heroName} />
      </NameTag>

      <div className="player-hero__meta">
        <span>
          Seen {formatDate(player.first_seen)} – {formatDate(player.last_seen)}
        </span>
        {player.total_playtime_seconds > 0 && (
          <span>{formatDuration(player.total_playtime_seconds)} played</span>
        )}
      </div>

      <div className="player-hero__headline">
        <HeadlineStat
          label="Matches"
          value={stats.completed_matches}
          title={
            stats.uncompleted_matches > 0
              ? `${stats.completed_matches} completed, ${stats.uncompleted_matches} incomplete`
              : undefined
          }
        />
        <HeadlineStat label="K/D" value={stats.kd_ratio.toFixed(2)} />
        <HeadlineStat label="Frags" value={stats.frags} />
        <HeadlineStat label="Deaths" value={stats.deaths} />
      </div>
    </header>
  );
}
