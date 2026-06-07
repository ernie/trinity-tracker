import type { ConnectionStatus } from "../useWebSocket";
import { derivePillState } from "./pillState";

interface StatusPillProps {
  humansOnline: number;
  activeServers: number;
  /** Hub WS state. Drives pulse; gray only when the feed is truly down. */
  status: ConnectionStatus;
  open: boolean;
  onToggle: () => void;
}

// Top-right live signal across non-landing routes. Mirrors .hero__pill.
export function StatusPill({
  humansOnline,
  activeServers,
  status,
  open,
  onToggle,
}: StatusPillProps) {
  const { stateClass, label } = derivePillState(status, humansOnline);

  return (
    <button
      type="button"
      className={`status-pill ${stateClass}`}
      onClick={onToggle}
      aria-haspopup="dialog"
      aria-expanded={open}
      aria-label={
        stateClass === "offline"
          ? "Disconnected from hub; open activity"
          : stateClass === "live"
            ? `${humansOnline} human${humansOnline === 1 ? "" : "s"} playing on ${activeServers} server${activeServers === 1 ? "" : "s"}; open activity`
            : "No humans playing; open activity"
      }
    >
      <span className="status-pill__dot" aria-hidden="true" />
      <span className="status-pill__label">{label}</span>
    </button>
  );
}
