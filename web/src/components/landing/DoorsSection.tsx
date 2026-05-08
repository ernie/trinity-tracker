import type React from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useFeaturedMatches, pickRandomFeatured } from '../../hooks/useFeaturedMatches'
import { DISCORD_INVITE_URL } from '../../constants/discord'

export function DoorsSection() {
  const { ids } = useFeaturedMatches()
  const navigate = useNavigate()

  const handleWatch = (e: React.MouseEvent) => {
    e.preventDefault()
    const id = pickRandomFeatured(ids)
    if (id !== null) {
      navigate(`/matches/${id}/demo`, { state: { from: '/' } })
    } else {
      navigate('/matches')
    }
  }

  return (
    <section className="landing-section landing-doors">
      <div className="landing-doors__grid">
        <Link to="/servers" className="landing-door">
          <h3 className="landing-door__title">Find a fight</h3>
          <p className="landing-door__desc">Live servers, current scores, who&rsquo;s on.</p>
          <span className="landing-door__arrow">All servers</span>
        </Link>

        <a href="/matches" onClick={handleWatch} className="landing-door">
          <h3 className="landing-door__title">Watch a fight</h3>
          <p className="landing-door__desc">Replay any match in your browser, frame by frame.</p>
          <span className="landing-door__arrow">Featured demo</span>
        </a>

        <Link to="/leaderboard" className="landing-door">
          <h3 className="landing-door__title">Pay respects</h3>
          <p className="landing-door__desc">Weekly rankings, all-time legends, every mode.</p>
          <span className="landing-door__arrow">Leaderboard</span>
        </Link>

        <a href={DISCORD_INVITE_URL} target="_blank" rel="noreferrer" className="landing-door">
          <h3 className="landing-door__title">Talk to the regulars</h3>
          <p className="landing-door__desc">Where everyone is when they&rsquo;re not in a server.</p>
          <span className="landing-door__arrow">Discord</span>
        </a>
      </div>
    </section>
  )
}
