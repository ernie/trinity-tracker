import { ArrowIcon } from "./ArrowIcon";
import type { SendKey, HoldHandlers } from "../hooks/useTvEngine";

// Free-camera movement pad, shared by the VOD demo player and the live player.
export function TvMovePad({
  sendKey,
  holdHandlers,
}: {
  sendKey: SendKey;
  holdHandlers: HoldHandlers;
}) {
  return (
    <div className="demo-move-pad" onContextMenu={(e) => e.preventDefault()}>
      <button
        className="move-btn"
        aria-label="Up"
        {...holdHandlers(
          () => sendKey("Space", "keydown"),
          () => sendKey("Space", "keyup"),
        )}
      >
        <ArrowIcon direction="up" size={16} />
      </button>
      <button
        className="move-btn"
        {...holdHandlers(
          () => sendKey("KeyW", "keydown"),
          () => sendKey("KeyW", "keyup"),
        )}
      >
        W
      </button>
      <button
        className="move-btn"
        aria-label="Down"
        {...holdHandlers(
          () => sendKey("KeyC", "keydown"),
          () => sendKey("KeyC", "keyup"),
        )}
      >
        <ArrowIcon direction="down" size={16} />
      </button>
      <button
        className="move-btn"
        {...holdHandlers(
          () => sendKey("KeyA", "keydown"),
          () => sendKey("KeyA", "keyup"),
        )}
      >
        A
      </button>
      <button
        className="move-btn"
        {...holdHandlers(
          () => sendKey("KeyS", "keydown"),
          () => sendKey("KeyS", "keyup"),
        )}
      >
        S
      </button>
      <button
        className="move-btn"
        {...holdHandlers(
          () => sendKey("KeyD", "keydown"),
          () => sendKey("KeyD", "keyup"),
        )}
      >
        D
      </button>
    </div>
  );
}
