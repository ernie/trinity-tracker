interface BotBadgeProps {
  isBot: boolean
  skill: number  // 1-5 skill level
  size?: 'sm' | 'md' | 'lg'
}

const SKILL_TITLES: Record<number, string> = {
  1: 'Bot - I Can Win',
  2: 'Bot - Bring It On',
  3: 'Bot - Hurt Me Plenty',
  4: 'Bot - Hardcore',
  5: 'Bot - Nightmare!',
}

export function BotBadge({ isBot, skill, size = 'sm' }: BotBadgeProps) {
  if (!isBot) {
    return null
  }

  const skillLevel = Number.isFinite(skill) ? Math.max(1, Math.min(5, Math.round(skill))) : 3
  const title = SKILL_TITLES[skillLevel]

  return (
    <span className={`bot-badge bot-badge-${size}`} title={title}>
      <img src={`/assets/skills/skill${skillLevel}.png`} alt={title} />
    </span>
  )
}
