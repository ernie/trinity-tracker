import type React from 'react'
import { Link } from 'react-router-dom'
import { useLiveData } from '../../contexts/LiveDataContext'
import { plural, formatFragTime } from './format'
import { HeroHeader } from './HeroHeader'

export function HeroSection() {
  const live = useLiveData()
  const humans = live.activeHumanPlayersCount
  const arenas = live.activeServersCount

  // Pulse line 1: busy / quiet / pre-WS variants.
  let pulse: React.ReactNode
  if (!live.isConnected) {
    pulse = <span>the arenas are open</span>
  } else if (humans === 0) {
    pulse = <span>the arenas are quiet. your move.</span>
  } else {
    pulse = (
      <span>
        <span className="landing-hero__pulse-dot" aria-hidden /> {humans}{' '}
        {plural(humans, 'player', 'players')} fragging across {arenas}{' '}
        {plural(arenas, 'arena', 'arenas')}
      </span>
    )
  }

  // Pulse line 2: matches today + frag duration. Stubbed in v1 — the underlying
  // values aren't on LiveDataContext yet. When `matchesToday === 0` the line
  // gracefully renders empty, so the hero still looks complete.
  // TODO(landing-page): wire matchesToday + fragSecondsToday from a derived hook
  //   or add the fields to LiveDataContext (compute from recentMatches today UTC,
  //   filtered to has_human_player && demo_available).
  const matchesToday = 0
  const fragTime = formatFragTime(0)
  const pulse2 = matchesToday > 0
    ? `${matchesToday} matches recorded today${fragTime ? ` · ${fragTime}` : ''}`
    : ''

  return (
    <section className="landing-hero">
      <picture className="landing-hero__bg" aria-hidden>
        <source
          type="image/avif"
          srcSet="
            /assets/landing/wallpaper-1200.avif 1200w,
            /assets/landing/wallpaper-1920.avif 1920w,
            /assets/landing/wallpaper-2880.avif 2880w"
          sizes="100vw"
        />
        <source
          type="image/webp"
          srcSet="
            /assets/landing/wallpaper-1200.webp 1200w,
            /assets/landing/wallpaper-1920.webp 1920w,
            /assets/landing/wallpaper-2880.webp 2880w"
          sizes="100vw"
        />
        <img src="/assets/landing/wallpaper.png" alt="" fetchPriority="high" />
      </picture>

      <HeroHeader />

      <div className="landing-hero__main">
        <div className="landing-hero__kicker">
          <span className="landing-hero__rule" aria-hidden />
          <span>EST. 2026 · TRINITY</span>
          <span className="landing-hero__rule" aria-hidden />
        </div>

        <h1 className="landing-hero__headline">
          WELCOME&nbsp;BACK&nbsp;TO<br />QUAKE&nbsp;III
        </h1>

        <p className="landing-hero__subhead">
          still the best arena shooter ever made.
        </p>

        <div className="landing-hero__pulse" aria-live="polite">
          <div className="landing-hero__pulse-line">{pulse}</div>
          {pulse2 && <div className="landing-hero__pulse-line2">{pulse2}</div>}
        </div>

        <div className="landing-hero__cta-row">
          <Link to="/docs/getting-started" className="landing-cta-primary">Enter the arena</Link>
          <Link to="/leaderboard" className="landing-cta-secondary">See the leaderboard →</Link>
        </div>
      </div>

      <div className="landing-hero__scroll-hint" aria-hidden>Scroll ↓</div>

      <div className="landing-hero__wallpaper" tabIndex={0}>
        <span className="landing-hero__wallpaper-trigger">Like this background?</span>
        <div className="landing-hero__wallpaper-popup" role="menu">
          <div className="landing-hero__wallpaper-popup-label">Download wallpaper</div>
          <a
            className="landing-hero__wallpaper-link"
            role="menuitem"
            href="/assets/landing/wallpaper.png"
            download="trinity-wallpaper-3440x1440.png"
          >
            <span>Ultrawide</span>
            <span className="landing-hero__wallpaper-link-meta">3440 × 1440</span>
          </a>
          <a
            className="landing-hero__wallpaper-link"
            role="menuitem"
            href="/assets/landing/wallpaper-qhd.png"
            download="trinity-wallpaper-2560x1440.png"
          >
            <span>QHD</span>
            <span className="landing-hero__wallpaper-link-meta">2560 × 1440</span>
          </a>
        </div>
      </div>
    </section>
  )
}
