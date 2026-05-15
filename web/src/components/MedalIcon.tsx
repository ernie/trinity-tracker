interface MedalIconProps {
  type: 'impressive' | 'excellent' | 'humiliation' | 'capture' | 'assist' | 'defend' | 'victory' | 'skull' | 'obelisk'
  count?: number
  size?: 'sm' | 'md' | 'lg'
  showCount?: boolean
  className?: string  // extra wrapper class (e.g. medal-icon--dim)
  title?: string      // override default tooltip
}

const MEDAL_FILES: Record<MedalIconProps['type'], string> = {
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

export function MedalIcon({ type, count, size = 'sm', showCount = true, className, title }: MedalIconProps) {
  const src = MEDAL_FILES[type]
  const resolvedTitle = title ?? MEDAL_TITLES[type]
  const sizeClass = SIZE_CLASSES[size]
  const cls = ['medal-icon', sizeClass, className].filter(Boolean).join(' ')
  const help = count && count > 1 ? `${MEDAL_HELP[type]} × ${count}` : MEDAL_HELP[type]

  return (
    <span className={cls} title={resolvedTitle} data-help={help}>
      <img src={src} alt={resolvedTitle} />
      {showCount && count && count > 1 && (
        <span className="medal-count">{count}</span>
      )}
    </span>
  )
}
