import type React from "react";
import { ModelHead } from "../ModelHead";
import { PlayerBadge } from "../PlayerBadge";
import { BotBadge } from "../BotBadge";
import { MedalIcon } from "../MedalIcon";
import { ColoredText } from "../ColoredText";
import { SCORE_HELP } from "./PlayerRows";
import type { ScoreState, AwardEntry } from "./format";
import { stripVRPrefix } from "../../utils";

export interface DuelistData {
  name: string;
  cleanName?: string;
  /** Model and skin, such as "sarge/default". */
  model?: string;
  isBot?: boolean;
  /** Bot skill, 1–5. */
  skill?: number;
  isVR?: boolean;
  isVerified?: boolean;
  isAdmin?: boolean;
  score: number;
  /** Text below the score, such as "24 ping" or "25 K · 19 D". */
  sub?: React.ReactNode;
  awards?: AwardEntry[];
  /** Opens PlayerStatsModal on click. */
  playerId?: number;
}

interface DuelistsProps {
  left: DuelistData;
  right: DuelistData;
  state: ScoreState;
  /** Live: always `vs`, with no winner or loser styling. */
  live?: boolean;
  onPlayerClick?: (
    playerName: string,
    cleanName: string,
    playerId?: number,
  ) => void;
  highlightPlayerId?: number;
}

function DuelistAwards({
  awards,
  side,
}: {
  awards?: AwardEntry[];
  side: "left" | "right";
}) {
  if (!awards || awards.length === 0) return null;
  return (
    <span
      className={`duelist-awards ${side === "right" ? "right" : ""}`}
      aria-hidden
    >
      {awards.map((a) => (
        <MedalIcon
          key={a.type}
          type={a.type}
          count={a.count}
          size="sm"
          team={a.team}
        />
      ))}
    </span>
  );
}

interface DuelistProps {
  data: DuelistData;
  side: "left" | "right";
  winnerSide: "left" | "right" | null;
  onClick?: (playerName: string, cleanName: string, playerId?: number) => void;
  highlighted: boolean;
}

function Duelist({
  data,
  side,
  winnerSide,
  onClick,
  highlighted,
}: DuelistProps) {
  const isWinner = winnerSide === side;
  const isLoser = winnerSide !== null && winnerSide !== side;
  const clickable = !!onClick && !data.isBot;
  const handleClick = clickable
    ? () => onClick!(data.name, data.cleanName ?? data.name, data.playerId)
    : undefined;

  return (
    <div
      className={`duelist ${clickable ? "clickable" : ""} ${highlighted ? "highlighted" : ""}`}
      onClick={handleClick}
      role={clickable ? "button" : undefined}
      tabIndex={clickable ? 0 : undefined}
      onKeyDown={
        clickable
          ? (e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                handleClick!();
              }
            }
          : undefined
      }
    >
      <span className={`duelist__portrait-wrap ${isWinner ? "winner" : ""}`}>
        <ModelHead model={data.model} size="lg" />
        {data.isBot ? (
          <BotBadge isBot skill={data.skill ?? 1} size="sm" />
        ) : (
          <PlayerBadge
            isVerified={data.isVerified}
            isAdmin={data.isAdmin}
            isVR={data.isVR}
            size="sm"
          />
        )}
        <DuelistAwards awards={data.awards} side={side} />
      </span>
      <span className={`duelist__name ${isLoser ? "dim" : ""}`}>
        <ColoredText text={data.isVR ? stripVRPrefix(data.name) : data.name} />
      </span>
      <span
        className={`duelist__score ${isLoser ? "loser" : ""} ${isWinner ? "winner" : ""}`}
        data-help={SCORE_HELP}
      >
        {data.score}
      </span>
      {data.sub && <span className="duelist__sub">{data.sub}</span>}
    </div>
  );
}

export function Duelists({
  left,
  right,
  state,
  live,
  onPlayerClick,
  highlightPlayerId,
}: DuelistsProps) {
  const winnerSide =
    !live && state === "left"
      ? "left"
      : !live && state === "right"
        ? "right"
        : null;

  // The portrait columns leave no room between them for a stacked
  // "NO CONTEST", so tie and no contest show as a banner above the row.
  const banner =
    !live && (state === "tie" || state === "no_contest") ? (
      <div className={`duelists-state ${state === "tie" ? "tie" : "nc"}`}>
        {state === "tie" ? "TIE" : "NO CONTEST"}
      </div>
    ) : null;

  return (
    <>
      {banner}
      <div className="duelists">
        <Duelist
          data={left}
          side="left"
          winnerSide={winnerSide}
          onClick={onPlayerClick}
          highlighted={
            highlightPlayerId !== undefined &&
            left.playerId === highlightPlayerId
          }
        />
        <span className="scoreboard__vs">vs</span>
        <Duelist
          data={right}
          side="right"
          winnerSide={winnerSide}
          onClick={onPlayerClick}
          highlighted={
            highlightPlayerId !== undefined &&
            right.playerId === highlightPlayerId
          }
        />
      </div>
    </>
  );
}
