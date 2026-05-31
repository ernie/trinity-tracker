// Crossed-banner emblem: blue behind, red in front, overlaid at opposing
// angles so the pair reads as a single composed mark rather than two
// icons side-by-side. Sized to fill its container — drop into a square
// parent for a balanced silhouette. Used by the leaderboard banner's
// flag_returns category and by the flag_returns honor cell + featured
// medal in PlayerHero / HonorsPanel.
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
