import { HeroSection } from './landing/HeroSection'
import { AboutSection } from './landing/AboutSection'
import { ActiveNowShelf } from './landing/ActiveNowShelf'
import { RecentMatchesShelf } from './landing/RecentMatchesShelf'

export function LandingPage() {
  return (
    <div className="landing">
      <HeroSection />
      <AboutSection />
      <ActiveNowShelf />
      <RecentMatchesShelf />
    </div>
  )
}
