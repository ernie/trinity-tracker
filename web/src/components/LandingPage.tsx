import { HeroSection } from './landing/HeroSection'
import { AboutSection } from './landing/AboutSection'
import { ActiveNowShelf } from './landing/ActiveNowShelf'

export function LandingPage() {
  return (
    <div className="landing">
      <HeroSection />
      <AboutSection />
      <ActiveNowShelf />
    </div>
  )
}
