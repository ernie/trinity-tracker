interface StatusPillProps {
  humansOnline: number
  activeServers: number
  /** Hub WebSocket connectivity. Drives the dot's pulse; even when no
   *  humans are on, a pulsing dot signals "page is live". */
  isConnected: boolean
  open: boolean
  onToggle: () => void
}

// Top-right live signal across non-landing routes. Mirrors the hero
// pill on the landing page: the dot pulses whenever the hub feed is
// connected, regardless of how many humans are playing. Label flips
// to "ALL QUIET" when nobody is on. Click toggles the activity drawer.
//
// (This replaces the older StatusPill + ConnectionStatus pair, where
// the green/red dot duplicated the connection signal that the pill
// itself can convey.)
export function StatusPill({ humansOnline, activeServers, isConnected, open, onToggle }: StatusPillProps) {
  const live = humansOnline > 0
  const label = live
    ? `${humansOnline} · ${activeServers} LIVE`
    : 'ALL QUIET'
  const stateClass = !isConnected ? 'offline' : live ? 'live' : 'quiet'

  return (
    <button
      type="button"
      className={`status-pill ${stateClass}`}
      onClick={onToggle}
      aria-haspopup="dialog"
      aria-expanded={open}
      aria-label={!isConnected
        ? 'Disconnected from hub; open activity'
        : live
          ? `${humansOnline} human${humansOnline === 1 ? '' : 's'} playing on ${activeServers} server${activeServers === 1 ? '' : 's'}; open activity`
          : 'No humans playing; open activity'}
    >
      <span className="status-pill__dot" aria-hidden="true" />
      <span className="status-pill__label">{label}</span>
    </button>
  )
}
