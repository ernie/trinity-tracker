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

  return (
    <span className={cls} title={resolvedTitle}>
      <img src={src} alt={resolvedTitle} />
      {showCount && count && count > 1 && (
        <span className="medal-count">{count}</span>
      )}
    </span>
  )
}
