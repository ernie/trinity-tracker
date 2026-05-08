import { HeroSection } from './landing/HeroSection'
import { AboutSection } from './landing/AboutSection'

export function LandingPage() {
  return (
    <div className="landing">
      <HeroSection />
      <AboutSection />
    </div>
  )
}
