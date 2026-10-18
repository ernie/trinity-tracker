import type React from "react";
import { EyeIcon } from "../EyeIcon";
import { stripVRPrefix } from "../../utils";

interface SpectatorStripProps {
  spectators: { name: string; isVR?: boolean; isNext?: boolean }[];
  isLive: boolean;
  /** Demo actions on finished cards. */
  rightSlot?: React.ReactNode;
  ariaLabel?: string;
}

export function SpectatorStrip({
  spectators,
  isLive,
  rightSlot,
  ariaLabel,
}: SpectatorStripProps) {
  const hasSpectators = spectators.length > 0;
  if (!hasSpectators && !rightSlot) return null;

  const eyeColorClass = isLive ? "" : "spec";
  const label =
    ariaLabel ?? (isLive ? "In queue or spectating" : "Spectated by");
  const help = isLive
    ? `Players spectating this server.
In Tournament, the next player in the queue plays the winner when the match ends.`
    : `Players who spectated this match.`;

  return (
    <div className="card__bottom" data-help={help}>
      {hasSpectators && (
        <span className="card__spectators">
          <span
            className={`card__spectators-label ${eyeColorClass}`}
            aria-label={label}
          >
            <EyeIcon />
          </span>
          <span className="card__spectators-list">
            {spectators.map((s, i) => (
              <span key={i} className={s.isNext ? "next" : ""}>
                {s.isVR ? stripVRPrefix(s.name) : s.name}
                {i < spectators.length - 1 ? " · " : ""}
              </span>
            ))}
          </span>
        </span>
      )}
      {rightSlot && <span className="card__bottom-actions">{rightSlot}</span>}
    </div>
  );
}
