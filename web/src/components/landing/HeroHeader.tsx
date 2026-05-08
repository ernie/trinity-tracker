// Bespoke header for the landing-page hero. Replaces the standard <Header>
// (which is a full app shell with PageNav, search, hamburger, login form) with
// a marketing-page treatment: brand on the left, sparse nav in the middle,
// live-count pill on the right. Sits over the wallpaper and scrolls away with
// the hero — no sticky transition.
import { Link } from 'react-router-dom'
import { useLiveData } from '../../contexts/LiveDataContext'

export function HeroHeader() {
  const { isConnected, activeHumanPlayersCount, activeServersCount } = useLiveData()
  const showPill = isConnected && (activeHumanPlayersCount > 0 || activeServersCount > 0)

  return (
    <header className="hero__header">
      <Link to="/" className="hero__brand">
        <img className="hero__brand-logo" src="/assets/icon-1104.png" alt="" aria-hidden />
        <span className="hero__brand-title">
          Trinity<span className="hero__brand-wordmark">tracker</span>
        </span>
      </Link>

      <nav className="hero__nav" aria-label="Primary">
        <Link to="/servers">Servers</Link>
        <Link to="/matches">Matches</Link>
        <Link to="/players">Players</Link>
        <Link to="/leaderboard">Leaderboard</Link>
        <Link to="/docs">Docs</Link>
      </nav>

      {showPill ? (
        <span className="hero__pill" aria-live="polite">
          <span className="dot" aria-hidden />
          {activeHumanPlayersCount} · {activeServersCount} LIVE
        </span>
      ) : (
        <span aria-hidden /> /* preserve grid third column */
      )}
    </header>
  )
}
