interface MedalIconProps {
  type: 'impressive' | 'excellent' | 'humiliation' | 'capture' | 'assist' | 'defend' | 'flag_return' | 'victory' | 'skull' | 'obelisk'
  count?: number
  size?: 'sm' | 'md' | 'lg'
  showCount?: boolean
  className?: string  // extra wrapper class (e.g. medal-icon--dim)
  title?: string      // override default tooltip
  /** For flag_return only: which team's flag to show. Defaults to neutral
   *  when the player has no team affiliation (shouldn't happen — returns
   *  only fire in CTF/1FCTF where everyone is on a team). */
  team?: 'red' | 'blue' | 'neutral'
}

const MEDAL_FILES: Record<Exclude<MedalIconProps['type'], 'flag_return'>, string> = {
  impressive: '/assets/medals/medal_impressive.png',
  excellent: '/assets/medals/medal_excellent.png',
  humiliation: '/assets/medals/medal_gauntlet.png',
  capture: '/assets/medals/medal_capture.png',
  assist: '/assets/medals/medal_assist.png',
  defend: '/assets/medals/medal_defend.png',
  victory: '/assets/medals/medal_victory.png',
  // Trinity-custom medals — shipped in pak3t.pk3 + pak8t.pk3,
  // extracted to /assets/medals/ by `trinity medals`.
  skull: '/assets/medals/medal_skull.png',
  obelisk: '/assets/medals/medal_obelisk.png',
}

const MEDAL_TITLES: Record<MedalIconProps['type'], string> = {
  impressive: 'Impressive',
  excellent: 'Excellent',
  humiliation: 'Humiliation',
  capture: 'Capture',
  assist: 'Assist',
  defend: 'Defense',
  flag_return: 'Flag return',
  victory: 'Victory',
  skull: 'Skulls delivered',
  obelisk: 'Obelisks destroyed',
}

// Longer per-type explanations consumed by HelpMode. The short
// MEDAL_TITLES above remain the native title= fallback; this map is
// what the rich popover shows on /docs.
const MEDAL_HELP: Record<MedalIconProps['type'], string> = {
  // Base Q3 frag-pattern medals — apply to every gametype with frags.
  excellent:   `Excellent — two frags within two seconds.`,
  impressive:  `Impressive — two consecutive railgun hits.`,
  humiliation: `Humiliation — gauntlet kill.`,
  // Objective-mode medals (primarily CTF / 1FCTF; assist also fires in TDM).
  capture:     `Capture — delivered the enemy flag.`,
  assist:      `Assist — helped a teammate get a frag or score an objective.`,
  defend:      `Defense — killed an enemy near your flag or carrier.`,
  // Trinity-tracked custom: the engine logs FlagReturn but never awarded
  // a medal for it. Auto-returns from timeout don't count.
  flag_return: `Flag return — touched your team's flag while it was loose, sending it back to base. Only player-initiated returns count.`,
  // Trinity-custom medals — shipped in pak3t.pk3 + pak8t.pk3,
  // extracted by `trinity medals`.
  skull:       `Skull delivery — Harvester score.`,
  obelisk:     `Obelisk destruction — Overload score.`,
  // Post-match only; renders on finished match cards, not live cards.
  victory:     `Match victory.`,
}

const SIZE_CLASSES: Record<NonNullable<MedalIconProps['size']>, string> = {
  sm: 'medal-icon-sm',
  md: 'medal-icon-md',
  lg: 'medal-icon-lg',
}

export function MedalIcon({ type, count, size = 'sm', showCount = true, className, title, team }: MedalIconProps) {
  const resolvedTitle = title ?? MEDAL_TITLES[type]
  const sizeClass = SIZE_CLASSES[size]
  const cls = ['medal-icon', sizeClass, className].filter(Boolean).join(' ')
  const help = count && count > 1 ? `${MEDAL_HELP[type]} × ${count}` : MEDAL_HELP[type]
  // flag_return uses the player's team flag PNG directly — same shape
  // and asset family as the FlagIcon used elsewhere, but lives inside
  // the medal strip's existing img-sizing rules so it scales like any
  // other medal without per-context overrides.
  const src = type === 'flag_return'
    ? `/assets/flags/flag_in_base_${team ?? 'neutral'}.png`
    : MEDAL_FILES[type]

  return (
    <span className={cls} title={resolvedTitle} data-help={help}>
      <img src={src} alt={resolvedTitle} />
      {showCount && count && count > 1 && (
        <span className="medal-count">{count}</span>
      )}
    </span>
  )
}
