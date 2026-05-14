import { BotBadge } from './BotBadge'
import { ColoredText } from './ColoredText'
import { PlayerPortrait } from './PlayerPortrait'
import { PlayerBadge } from './PlayerBadge'
import { HeadlineStat } from './HeadlineStat'
import { HONORS, DEFAULT_FEATURED_HONOR_KEY } from '../constants/honors'
import { displayPlayerName } from '../utils'
import { formatDate, formatDuration } from '../utils/formatters'
import type { PlayerStatsResponse } from '../types'

interface PlayerHeroProps {
  player: PlayerStatsResponse['player']
  stats: PlayerStatsResponse['stats']
  /** 'modal' is the compact 80px-portrait variant; 'page' is the
   *  120px hero used on /players/:id. CSS modifier classes drive the
   *  scale; markup is identical. */
  variant: 'modal' | 'page'
  /** Optional: shown if the fetched player record has an empty name
   *  (race during initial load). Only relevant in the modal — the
   *  full-profile page already has stats by the time it renders. */
  fallbackName?: string
}

// Player hero: portrait + featured-honor card paired at the top,
// name + meta line, then a 4-up headline strip (Matches · K/D · Frags ·
// Deaths). The featured honor defaults to Victory; the owning user can
// promote any other honor via the star buttons in HonorsPanel. Shared
// between PlayerStatsModal (modal variant) and PlayersPage (page variant).
export function PlayerHero({ player, stats, variant, fallbackName }: PlayerHeroProps) {
  const heroName = displayPlayerName(player) || fallbackName || ''
  const NameTag = variant === 'modal' ? 'h3' : 'h2'
  // Resolve featured honor by key, falling back to Victory if the linked
  // user hasn't chosen, there's no linked user, or the stored key drifted
  // out of HONORS (e.g. a key was retired without a data backfill).
  const featuredKey = player.featured_honor ?? DEFAULT_FEATURED_HONOR_KEY
  const featured = HONORS.find((h) => h.key === featuredKey) ?? HONORS[0]

  return (
    <header className={`player-hero player-hero--${variant}`}>
      <div className="player-hero__top">
        <div className="player-hero__portrait">
          <PlayerPortrait model={player.model} size="lg" />
          {player.is_bot ? (
            <BotBadge isBot skill={5} size={variant === 'page' ? 'lg' : 'md'} />
          ) : (
            <PlayerBadge
              isVerified={player.is_verified}
              isAdmin={player.is_admin}
              isVR={player.is_vr}
              size={variant === 'page' ? 'lg' : 'md'}
            />
          )}
        </div>
        <div className="player-hero__featured" title={featured.label}>
          <img className="player-hero__featured-medal" src={featured.icon} alt={featured.label} />
          <span className="player-hero__featured-count">{featured.value(stats)}</span>
        </div>
      </div>

      <NameTag className="player-hero__name">
        <ColoredText text={heroName} />
      </NameTag>

      <div className="player-hero__meta">
        <span>Seen {formatDate(player.first_seen)} – {formatDate(player.last_seen)}</span>
        {player.total_playtime_seconds > 0 && (
          <span>{formatDuration(player.total_playtime_seconds)} played</span>
        )}
      </div>

      <div className="player-hero__headline">
        <HeadlineStat
          label="Matches"
          value={stats.completed_matches}
          title={stats.uncompleted_matches > 0
            ? `${stats.completed_matches} completed, ${stats.uncompleted_matches} incomplete`
            : undefined}
        />
        <HeadlineStat label="K/D" value={stats.kd_ratio.toFixed(2)} />
        <HeadlineStat label="Frags" value={stats.frags} />
        <HeadlineStat label="Deaths" value={stats.deaths} />
      </div>
    </header>
  )
}
