// Hero greeting variants — one is picked at random on each HeroSection
// mount to keep the landing page lively. Mix of two voice registers:
//
//   world-speaks   — the arenas / lights / gauntlet / portal as subject,
//                    subhead pivots to the listener
//   declarative-pride — proud assertion about the game, subhead invites
//
// Constraints worked into the copy: each headline follows the 3-word top
// + 2-word bottom cadence that matches "WELCOME BACK TO / QUAKE III", and
// top-line character counts stay within ~18 chars so the Cinzel 700
// display at the largest hero size doesn't shrink or wrap awkwardly.

export type HeroVariant = {
  headline1: string  // first line of <h1>, rendered uppercase via content
  headline2: string  // second line of <h1>
  subhead: string    // <p> body — CSS auto-lowercases via text-transform
}

export const HERO_VARIANTS: readonly HeroVariant[] = [
  { headline1: 'THE ARENAS ARE',     headline2: 'STILL HERE',     subhead: 'it was you that walked away.' },
  { headline1: 'THE LIGHTS WERE',    headline2: 'NEVER OFF',      subhead: 'you simply forgot how to find them.' },
  { headline1: 'THE ARENAS REMAIN',  headline2: 'YOU LEFT',       subhead: "but it's never too late to return." },
  { headline1: 'THE GAUNTLET WAITS', headline2: 'STILL HUMMING',  subhead: 'for hands that remember its weight.' },
  { headline1: 'THE PORTAL STILL',   headline2: 'SPINS OPEN',     subhead: "step through whenever you're ready." },
  { headline1: 'STILL THE BEST',     headline2: 'SINCE 1999',     subhead: 'join us, and remember why.' },
  { headline1: 'THE EMBERS GLOW',    headline2: 'SINCE 1999',     subhead: 'come reignite the flame.' },
  { headline1: 'THE ACCURACY IS',    headline2: 'YOURS ALONE',    subhead: "you've never needed an assist." },
  { headline1: 'WELCOME BACK TO',    headline2: 'QUAKE III',      subhead: 'still the best arena shooter ever made.' },
] as const

export function pickHeroVariant(): HeroVariant {
  return HERO_VARIANTS[Math.floor(Math.random() * HERO_VARIANTS.length)]
}
