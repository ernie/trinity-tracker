interface StatusPillProps {
  humansOnline: number;
  activeServers: number;
  /** Hub WS connectivity. Drives pulse; gray when disconnected. */
  isConnected: boolean;
  open: boolean;
  onToggle: () => void;
}

// Top-right live signal across non-landing routes. Mirrors .hero__pill.
export function StatusPill({
  humansOnline,
  activeServers,
  isConnected,
  open,
  onToggle,
}: StatusPillProps) {
  const live = humansOnline > 0;
  const stateClass = !isConnected ? "offline" : live ? "live" : "quiet";
  const label = !isConnected
    ? "OFFLINE"
    : live
      ? `${humansOnline} LIVE`
      : "STANDING BY";

  return (
    <button
      type="button"
      className={`status-pill ${stateClass}`}
      onClick={onToggle}
      aria-haspopup="dialog"
      aria-expanded={open}
      aria-label={
        !isConnected
          ? "Disconnected from hub; open activity"
          : live
            ? `${humansOnline} human${humansOnline === 1 ? "" : "s"} playing on ${activeServers} server${activeServers === 1 ? "" : "s"}; open activity`
            : "No humans playing; open activity"
      }
    >
      <span className="status-pill__dot" aria-hidden="true" />
      <span className="status-pill__label">{label}</span>
    </button>
  );
}
