import type React from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { DISCORD_INVITE_URL } from '../../constants/discord'
import { DocsH2 } from './DocsH2'
import { ArrowIcon } from '../ArrowIcon'
import { useFeaturedMatches, pickRandomFeatured } from '../../hooks/useFeaturedMatches'

// New /docs index. Replaces the previous redirect to
// /docs/getting-started. First-time visitors see this after picking
// a platform; the persona cards route them to the right deeper
// surface.
export function DocsWelcome() {
  const { ids } = useFeaturedMatches()
  const navigate = useNavigate()

  // Mirrors the landing page's "Watch a fight" door — picks a random
  // featured match and opens its demo player. Falls back to /matches
  // if the featured pool is empty (e.g., a fresh deploy with nothing
  // flagged yet).
  const handleWatchFeatured = (e: React.MouseEvent) => {
    e.preventDefault()
    const id = pickRandomFeatured(ids)
    if (id !== null) {
      navigate(`/matches/${id}/demo`, { state: { from: '/docs' } })
    } else {
      navigate('/matches')
    }
  }

  return (
    <>
      <div className="about-section docs-welcome__hero">
        <h1 className="docs-welcome__title">Welcome to Trinity</h1>
        <p className="docs-welcome__lead">
          Trinity is Quake 3 with modern conveniences — VR, voice chat,
          and shared stats — built on top of the original game you know.
          It's backwards-compatible with vanilla Q3 servers, so installing
          it never costs you the rest of the community.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="where-to-start">Where to start</DocsH2>
        <div className="docs-welcome__personas">
          <Link to="/features" className="docs-welcome__persona-card">
            <span className="docs-welcome__persona-title">What's in Trinity?</span>
            <span className="docs-welcome__persona-teaser">
              Watch the features in action — VR tracking, voice chat,
              demo playback, and more.
            </span>
            <ArrowIcon direction="right" size={14} className="docs-welcome__persona-arrow" />
          </Link>

          <Link to="/docs/install" className="docs-welcome__persona-card">
            <span className="docs-welcome__persona-title">I'm ready to install</span>
            <span className="docs-welcome__persona-teaser">
              Download an engine, set up your config, claim a Trinity
              account, and start playing.
            </span>
            <ArrowIcon direction="right" size={14} className="docs-welcome__persona-arrow" />
          </Link>

          <Link to="/docs/server-admin" className="docs-welcome__persona-card">
            <span className="docs-welcome__persona-title">I run a Q3 server</span>
            <span className="docs-welcome__persona-teaser">
              Set up the Trinity collector and configure your server to
              participate in the network.
            </span>
            <ArrowIcon direction="right" size={14} className="docs-welcome__persona-arrow" />
          </Link>
        </div>
      </div>

      <div className="about-section">
        <DocsH2 id="watch-featured">Watch a featured Trinity match</DocsH2>
        <p>
          Curious before you commit?{' '}
          <a href="/matches" onClick={handleWatchFeatured}>
            Watch a match right here
          </a>{' '}
          — replays a real Trinity match frame by frame, no install required.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="get-help">Get help, get involved</DocsH2>
        <p>
          Stuck on something or just want to say hi?{' '}
          <a href={DISCORD_INVITE_URL}>Trinity Discord</a> is the place.
        </p>
      </div>
    </>
  )
}
