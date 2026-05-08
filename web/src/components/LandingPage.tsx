import { HeroSection } from './landing/HeroSection'
import { AboutSection } from './landing/AboutSection'
import { ActiveNowShelf } from './landing/ActiveNowShelf'
import { RecentMatchesShelf } from './landing/RecentMatchesShelf'
import { TopPlayersStrip } from './landing/TopPlayersStrip'

export function LandingPage() {
  return (
    <div className="landing">
      <HeroSection />
      <AboutSection />
      <ActiveNowShelf />
      <RecentMatchesShelf />
      <TopPlayersStrip />
    </div>
  )
}
