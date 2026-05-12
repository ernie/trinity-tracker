// Hero greeting variants — one is picked at random on each HeroSection
// mount to keep the landing page lively.
//
// Typographic constraints: headlines mostly follow a 3-word top + 2-word
// bottom cadence, with top lines within ~18 chars so the Cinzel 700
// display at the largest hero size doesn't shrink or wrap. The CTF
// variant ("HOME IS WHERE / YOUR FLAG IS") deliberately breaks the 5-word
// rule with a 3+3 couplet.

export type HeroVariant = {
  headline1: string; // first line of <h1>, rendered uppercase via content
  headline2: string; // second line of <h1>
  subhead: string; // <p> body — CSS auto-lowercases via text-transform
};

// Quake III Arena shipped 1999-12-02. Use full elapsed years so the line
// only ticks up on the anniversary, not at January 1st.
const QUAKE3_RELEASE = new Date(1999, 11, 2);

function fullYearsSince(date: Date, now: Date = new Date()): number {
  let years = now.getFullYear() - date.getFullYear();
  const beforeAnniversary =
    now.getMonth() < date.getMonth() ||
    (now.getMonth() === date.getMonth() && now.getDate() < date.getDate());
  if (beforeAnniversary) years -= 1;
  return years;
}

export const HERO_VARIANTS: readonly HeroVariant[] = [
  {
    headline1: "THE RAILGUN WAITS,",
    headline2: "STILL HUMMING",
    subhead: "it knows you're impressive — prove it right.",
  },
  {
    headline1: "YOU DON'T ALWAYS",
    headline2: "NEED BULLETS",
    subhead: "humiliation is the best revenge.",
  },
  {
    headline1: `${fullYearsSince(QUAKE3_RELEASE)} YEARS LATER,`,
    headline2: "EMBERS GLOW",
    subhead: "the servers never went cold.",
  },
  {
    headline1: "THE ACCURACY IS",
    headline2: "YOURS ALONE",
    subhead: "you've never needed an assist.",
  },
  {
    headline1: "WELCOME BACK TO",
    headline2: "QUAKE III",
    subhead: "the arena missed your footsteps.",
  },
  {
    headline1: "NO LOADOUTS AND",
    headline2: "NO COOLDOWNS",
    subhead: "the arena asks more of you.",
  },
  {
    headline1: "YOU KNOW WHAT",
    headline2: "EXCELLENT IS",
    subhead: "two frags in two seconds.",
  },
  {
    headline1: "PICK A SERVER",
    headline2: "AND FIGHT",
    subhead: "earn a nemesis.",
  },
  {
    headline1: "HOME IS WHERE",
    headline2: "YOUR FLAG IS",
    subhead: "it's not camping, it's defending.",
  },
  {
    headline1: "BRING YOUR FRIENDS",
    headline2: "(OR ENEMIES)",
    subhead: "the arena will sort them out.",
  },
] as const;

export function pickHeroVariant(): HeroVariant {
  return HERO_VARIANTS[Math.floor(Math.random() * HERO_VARIANTS.length)];
}
