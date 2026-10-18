import type React from "react";
import { useState } from "react";
import { ModelHead } from "../ModelHead";
import { PlayerBadge } from "../PlayerBadge";
import { BotBadge } from "../BotBadge";
import { MedalIcon } from "../MedalIcon";
import { ColoredText } from "../ColoredText";
import type { AwardEntry } from "./format";
import { SCORE_HELP } from "./PlayerRows";
import { stripVRPrefix } from "../../utils";

export interface FfaHeroPlayer {
  /** client_num on live cards, player_id on finished ones; two players
   *  can share a name. */
  key: string | number;
  name: string;
  cleanName?: string;
  model?: string;
  isBot?: boolean;
  skill?: number;
  isVR?: boolean;
  isVerified?: boolean;
  isAdmin?: boolean;
  score: number;
  /** Secondary text: `{ping} ping` live, `{frags} K · {deaths} D` finished. */
  sub?: React.ReactNode;
  awards?: AwardEntry[];
  playerId?: number;
}

interface FfaHeroProps {
  /** Null shows a placeholder of the same height until the first frag. */
  player: FfaHeroPlayer | null;
  /** Gold portrait frame and score; the caller decides, since a tie has no winner. */
  gildScore?: boolean;
  onPlayerClick?: (
    playerName: string,
    cleanName: string,
    playerId?: number,
  ) => void;
}

export function FfaHero({ player, gildScore, onPlayerClick }: FfaHeroProps) {
  if (!player) {
    return (
      <div className="ffa-hero ffa-hero--placeholder" aria-hidden>
        <span className="duelist__portrait-wrap ffa-hero__portrait-wrap">
          <img
            className="ffa-hero__placeholder-icon"
            src="/assets/q3-logo.png"
            alt=""
          />
        </span>
        <div className="ffa-hero__identity">
          <span className="ffa-hero__name">Awaiting first blood</span>
        </div>
      </div>
    );
  }
  const clickable = !!onPlayerClick && !player.isBot;
  const handleClick = clickable
    ? () =>
        onPlayerClick!(
          player.name,
          player.cleanName ?? player.name,
          player.playerId,
        )
    : undefined;
  return (
    <div
      className={`ffa-hero ${clickable ? "clickable" : ""}`}
      onClick={handleClick}
      role={clickable ? "button" : undefined}
      tabIndex={clickable ? 0 : undefined}
      onKeyDown={
        clickable
          ? (e: React.KeyboardEvent) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                handleClick!();
              }
            }
          : undefined
      }
    >
      <span
        className={`duelist__portrait-wrap ffa-hero__portrait-wrap ${gildScore ? "winner" : ""}`}
      >
        <ModelHead model={player.model} size="lg" />
        {player.isBot ? (
          <BotBadge isBot skill={player.skill ?? 1} size="sm" />
        ) : (
          <PlayerBadge
            isVerified={player.isVerified}
            isAdmin={player.isAdmin}
            isVR={player.isVR}
            size="sm"
          />
        )}
      </span>
      <div className="ffa-hero__identity">
        <span className="ffa-hero__name">
          <ColoredText
            text={player.isVR ? stripVRPrefix(player.name) : player.name}
          />
        </span>
        <span
          className={`ffa-hero__score ${gildScore ? "winner" : ""}`}
          data-help={SCORE_HELP}
        >
          {player.score}
        </span>
        <span className="ffa-hero__medals">
          {(player.awards ?? []).map((a) => (
            <MedalIcon
              key={a.type}
              type={a.type}
              count={a.count}
              size="sm"
              team={a.team}
            />
          ))}
        </span>
        {player.sub && <span className="ffa-hero__sub">{player.sub}</span>}
      </div>
    </div>
  );
}

/** The first player to reach each new high score holds the hero slot
 *  until someone passes them; a tie keeps the incumbent. Returns null
 *  until the first frag. If the incumbent leaves, the current top scorer
 *  takes over, since who reached that score first is unknown. */
export function useFfaHero<P extends FfaHeroPlayer>(players: P[]): P | null {
  const [incumbent, setIncumbent] = useState<{
    key: string | number;
    score: number;
  } | null>(null);
  const scored = players.filter((p) => p.score > 0);

  let hero: P | null = null;
  if (scored.length > 0) {
    const found = incumbent
      ? scored.find((p) => p.key === incumbent.key)
      : undefined;
    if (found) {
      const usurper = scored.find(
        (p) => p.key !== incumbent!.key && p.score > found.score,
      );
      hero = usurper ?? found;
    } else {
      hero = [...scored].sort((a, b) => b.score - a.score)[0];
    }
  }

  // Set during render only on a change, so the render settles.
  const nextKey = hero ? hero.key : null;
  const nextScore = hero?.score ?? 0;
  const changed =
    (incumbent?.key ?? null) !== nextKey ||
    (incumbent?.score ?? 0) !== nextScore;
  if (changed) {
    setIncumbent(hero ? { key: nextKey!, score: nextScore } : null);
  }

  return hero;
}
