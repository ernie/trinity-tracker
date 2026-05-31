import type React from "react";
import type { ScoreState } from "./format";

// Scoreboard tooltip — appears on every team-mode card (TDM, CTF,
// 1FCTF, Overload, Harvester). Mode-agnostic phrasing; per-mode
// indicators in the slots get their own data-help on the slot
// elements (added in their respective ServerCard render branches).
const SCOREBOARD_HELP = `Team scoreboard. Each side shows the team's current score.
Score meaning depends on mode — frags (TDM), captures (CTF, 1FCTF), obelisks destroyed (Overload), or skulls delivered (Harvester).`;

interface ScoreboardProps {
  redLabel: string;
  redScore: number;
  blueLabel: string;
  blueScore: number;
  state: ScoreState; // 'left' | 'right' | 'tie' | 'no_contest'
  /** Live: always `vs`, no winner/loser dimming. */
  live?: boolean;
  /** Slot next to red score — CTF flag indicator. */
  redIndicator?: React.ReactNode;
  /** Slot next to blue score — CTF flag indicator. */
  blueIndicator?: React.ReactNode;
  /** Slot below `vs` — 1FCTF neutral flag drift. */
  centerIndicator?: React.ReactNode;
}

export function Scoreboard({
  redLabel,
  redScore,
  blueLabel,
  blueScore,
  state,
  live,
  redIndicator,
  blueIndicator,
  centerIndicator,
}: ScoreboardProps) {
  // Live cards skip winner/loser hooks — outcome isn't decided.
  const redClass =
    !live && state === "left"
      ? "winner"
      : !live && state === "right"
        ? "loser"
        : "";
  const blueClass =
    !live && state === "right"
      ? "winner"
      : !live && state === "left"
        ? "loser"
        : "";

  let connector: React.ReactNode = "vs";
  if (!live && state === "tie")
    connector = <span className="scoreboard__state tie">TIE</span>;
  if (!live && state === "no_contest")
    connector = (
      <span className="scoreboard__state nc">
        NO
        <br />
        CONTEST
      </span>
    );

  return (
    <div className="scoreboard" data-help={SCOREBOARD_HELP}>
      <div className="scoreboard__side scoreboard__side--red">
        <span className="scoreboard__label red">{redLabel}</span>
        <span className="scoreboard__score-line">
          <span className={`scoreboard__score ${redClass}`}>{redScore}</span>
          {redIndicator && (
            <span className="scoreboard__indicator">{redIndicator}</span>
          )}
        </span>
      </div>
      <div className="scoreboard__center">
        <span className="scoreboard__vs">{connector}</span>
        {centerIndicator && (
          <span className="scoreboard__center-indicator">
            {centerIndicator}
          </span>
        )}
      </div>
      <div className="scoreboard__side scoreboard__side--blue">
        <span className="scoreboard__label blue">{blueLabel}</span>
        <span className="scoreboard__score-line">
          {blueIndicator && (
            <span className="scoreboard__indicator">{blueIndicator}</span>
          )}
          <span className={`scoreboard__score ${blueClass}`}>{blueScore}</span>
        </span>
      </div>
    </div>
  );
}
