interface MedalIconProps {
  type: 'impressive' | 'excellent' | 'humiliation' | 'capture' | 'assist' | 'defend' | 'victory'
  count?: number
  size?: 'sm' | 'md' | 'lg'
  showCount?: boolean
}

const MEDAL_FILES: Record<MedalIconProps['type'], string> = {
  impressive: '/assets/medals/medal_impressive.png',
  excellent: '/assets/medals/medal_excellent.png',
  humiliation: '/assets/medals/medal_gauntlet.png',
  capture: '/assets/medals/medal_capture.png',
  assist: '/assets/medals/medal_assist.png',
  defend: '/assets/medals/medal_defend.png',
  victory: '/assets/medals/medal_victory.png',
}

const MEDAL_TITLES: Record<MedalIconProps['type'], string> = {
  impressive: 'Impressive',
  excellent: 'Excellent',
  humiliation: 'Humiliation',
  capture: 'Capture',
  assist: 'Assist',
  defend: 'Defense',
  victory: 'Victory',
}

const SIZE_CLASSES: Record<NonNullable<MedalIconProps['size']>, string> = {
  sm: 'medal-icon-sm',
  md: 'medal-icon-md',
  lg: 'medal-icon-lg',
}

export function MedalIcon({ type, count, size = 'sm', showCount = true }: MedalIconProps) {
  const src = MEDAL_FILES[type]
  const title = MEDAL_TITLES[type]
  const sizeClass = SIZE_CLASSES[size]

  return (
    <span className={`medal-icon ${sizeClass}`} title={title}>
      <img src={src} alt={title} />
      {showCount && count && count > 1 && (
        <span className="medal-count">{count}</span>
      )}
    </span>
  )
}
