interface StatusPillProps {
  humansOnline: number
  activeServers: number
  open: boolean
  onToggle: () => void
}

// Brand-row live signal. Reads aggregated counts derived in App from
// the live server map. Pulses when any humans are on; otherwise sits
// dim with the "all quiet" copy. Click toggles the activity drawer.
export function StatusPill({ humansOnline, activeServers, open, onToggle }: StatusPillProps) {
  const live = humansOnline > 0
  const label = live
    ? `${humansOnline} on · ${activeServers} server${activeServers === 1 ? '' : 's'}`
    : 'all quiet'

  return (
    <button
      type="button"
      className={`status-pill ${live ? 'live' : 'quiet'}`}
      onClick={onToggle}
      aria-haspopup="dialog"
      aria-expanded={open}
      aria-label={live
        ? `${humansOnline} human${humansOnline === 1 ? '' : 's'} playing on ${activeServers} server${activeServers === 1 ? '' : 's'}; open activity`
        : 'No humans playing; open activity'}
    >
      <span className="status-pill__dot" aria-hidden="true" />
      <span className="status-pill__label">{label}</span>
    </button>
  )
}
