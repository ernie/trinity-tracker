interface BotBadgeProps {
  isBot: boolean;
  skill: number; // 1-5 skill level
  size?: "sm" | "md" | "lg";
}

const SKILL_TITLES: Record<number, string> = {
  1: "Bot - I Can Win",
  2: "Bot - Bring It On",
  3: "Bot - Hurt Me Plenty",
  4: "Bot - Hardcore",
  5: "Bot - Nightmare!",
};

// HelpMode text — describes the badge as a taxonomy rather than the
// specific skill of the bot in front of the reader. The native title=
// stays per-skill so hover-tooltips on /servers still tell you which
// tier this specific bot is at.
const BOT_HELP = `Bot. Icon indicates difficulty:
• I Can Win (easy)
• Bring It On
• Hurt Me Plenty
• Hardcore
• Nightmare! (max)`;

export function BotBadge({ isBot, skill, size = "sm" }: BotBadgeProps) {
  if (!isBot) {
    return null;
  }

  const skillLevel = Number.isFinite(skill)
    ? Math.max(1, Math.min(5, Math.round(skill)))
    : 3;
  const title = SKILL_TITLES[skillLevel];

  return (
    <span
      className={`bot-badge bot-badge-${size}`}
      title={title}
      data-help={BOT_HELP}
    >
      <img src={`/assets/skills/skill${skillLevel}.png`} alt={title} />
    </span>
  );
}
