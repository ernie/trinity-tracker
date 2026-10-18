import type React from "react";
import type { ScoreState } from "./format";

const SCOREBOARD_HELP = `Team scores.
What scores depends on the game type: frags in TDM, captures in CTF and One Flag CTF, obelisks destroyed in Overload, skulls delivered in Harvester.`;

interface ScoreboardProps {
  redLabel: string;
  redScore: number;
  blueLabel: string;
  blueScore: number;
  state: ScoreState;
  /** Live: always `vs`, with no winner or loser styling. */
  live?: boolean;
  /** Beside the red score, such as the CTF flag status. */
  redIndicator?: React.ReactNode;
  /** Beside the blue score, such as the CTF flag status. */
  blueIndicator?: React.ReactNode;
  /** Below `vs`, such as the One Flag CTF neutral flag. */
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
