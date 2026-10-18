// Fills its container, so give it a square parent.
export function FlagPair({ className = "" }: { className?: string }) {
  return (
    <span className={`flag-pair ${className}`.trim()} aria-hidden>
      <img
        className="flag-pair__blue"
        src="/assets/flags/flag_in_base_blue.png"
        alt=""
      />
      <img
        className="flag-pair__red"
        src="/assets/flags/flag_in_base_red.png"
        alt=""
      />
    </span>
  );
}
