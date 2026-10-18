interface MedalIconProps {
  type:
    | "impressive"
    | "excellent"
    | "humiliation"
    | "capture"
    | "assist"
    | "defend"
    | "flag_return"
    | "victory"
    | "skull"
    | "obelisk";
  count?: number;
  size?: "sm" | "md" | "lg";
  showCount?: boolean;
  className?: string;
  title?: string;
  /** For flag_return only: which team's flag to show. Defaults to neutral. */
  team?: "red" | "blue" | "neutral";
}

const MEDAL_FILES: Record<
  Exclude<MedalIconProps["type"], "flag_return">,
  string
> = {
  impressive: "/assets/medals/medal_impressive.png",
  excellent: "/assets/medals/medal_excellent.png",
  humiliation: "/assets/medals/medal_gauntlet.png",
  capture: "/assets/medals/medal_capture.png",
  assist: "/assets/medals/medal_assist.png",
  defend: "/assets/medals/medal_defend.png",
  victory: "/assets/medals/medal_victory.png",
  // `trinity medals` extracts these two from the Trinity paks.
  skull: "/assets/medals/medal_skull.png",
  obelisk: "/assets/medals/medal_obelisk.png",
};

const MEDAL_TITLES: Record<MedalIconProps["type"], string> = {
  impressive: "Impressive",
  excellent: "Excellent",
  humiliation: "Humiliation",
  capture: "Capture",
  assist: "Assist",
  defend: "Defense",
  flag_return: "Flag return",
  victory: "Victory",
  skull: "Skulls delivered",
  obelisk: "Obelisks destroyed",
};

const MEDAL_HELP: Record<MedalIconProps["type"], string> = {
  excellent: `Excellent: two frags within three seconds.`,
  impressive: `Impressive: two railgun hits in a row.`,
  humiliation: `Humiliation: a gauntlet frag.`,
  capture: `Capture: brought the enemy flag home.`,
  assist: `Assist: helped your team score a capture, a skull delivery or an obelisk kill.`,
  defend: `Defense: fragged an enemy near your flag or your carrier.`,
  flag_return: `Flag return: touched your team's dropped flag to send it back to base.`,
  skull: `Skull delivery: scored skulls in Harvester.`,
  obelisk: `Obelisk destruction: destroyed the enemy obelisk in Overload.`,
  victory: `Victory: won the match.`,
};

const SIZE_CLASSES: Record<NonNullable<MedalIconProps["size"]>, string> = {
  sm: "medal-icon-sm",
  md: "medal-icon-md",
  lg: "medal-icon-lg",
};

export function MedalIcon({
  type,
  count,
  size = "sm",
  showCount = true,
  className,
  title,
  team,
}: MedalIconProps) {
  const resolvedTitle = title ?? MEDAL_TITLES[type];
  const sizeClass = SIZE_CLASSES[size];
  const cls = ["medal-icon", sizeClass, className].filter(Boolean).join(" ");
  const help =
    count && count > 1 ? `${MEDAL_HELP[type]} × ${count}` : MEDAL_HELP[type];
  const src =
    type === "flag_return"
      ? `/assets/flags/flag_in_base_${team ?? "neutral"}.png`
      : MEDAL_FILES[type];

  return (
    <span className={cls} title={resolvedTitle} data-help={help}>
      <img src={src} alt={resolvedTitle} />
      {showCount && count && count > 1 && (
        <span className="medal-count">{count}</span>
      )}
    </span>
  );
}
