// Bottom-of-card strip with eye icon + spectator/queue list. Shows
// ember-light when live (current activity, matters now); text-dim when
// historical. Empty state hides the left side; right side (demo
// actions on finished cards) right-aligns naturally.
import type React from "react";
import { EyeIcon } from "../EyeIcon";
import { stripVRPrefix } from "../../utils";

interface SpectatorStripProps {
  spectators: { name: string; isVR?: boolean; isNext?: boolean }[];
  isLive: boolean;
  /** Demo actions render to the right on finished cards. Provide as JSX. */
  rightSlot?: React.ReactNode;
  /** ARIA label override (defaults to "Spectating" / "In queue" / "Spectated by"). */
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
    ? `Players currently spectating this server.
In Tournament (1v1), the next in queue plays the winner when a match ends.`
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
