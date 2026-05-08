import { useEffect, useRef, useState } from 'react'
import { Header } from './Header'
import { HeroSection } from './landing/HeroSection'
import { AboutSection } from './landing/AboutSection'
import { ActiveNowShelf } from './landing/ActiveNowShelf'
import { RecentMatchesShelf } from './landing/RecentMatchesShelf'
import { TopPlayersStrip } from './landing/TopPlayersStrip'
import { DoorsSection } from './landing/DoorsSection'

export function LandingPage() {
  const heroSentinel = useRef<HTMLDivElement>(null)
  const [solid, setSolid] = useState(false)

  useEffect(() => {
    const el = heroSentinel.current
    if (!el) return
    // When the sentinel (placed ~80% down the hero) leaves the viewport top,
    // we have scrolled past the hero — flip the header to its solid variant.
    const obs = new IntersectionObserver(
      ([entry]) => setSolid(!entry.isIntersecting),
      { rootMargin: '-1px 0px 0px 0px', threshold: 0 },
    )
    obs.observe(el)
    return () => obs.disconnect()
  }, [])

  return (
    <div className="landing">
      <Header
        title="Trinity"
        className="app-header"
        wordmark="tracker"
        transparent
        solid={solid}
        linkToHome={false}
      />
      <HeroSection sentinelRef={heroSentinel} />
      <AboutSection />
      <ActiveNowShelf />
      <RecentMatchesShelf />
      <TopPlayersStrip />
      <DoorsSection />
    </div>
  )
}
