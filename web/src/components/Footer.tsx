import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { DISCORD_INVITE_URL } from '../constants/discord'
import { GITHUB_REPO_URL } from '../constants/github'

// Persistent footer band on every route. Surfaces the binary's version
// (fetched from /api/version), external community links, and authorship.
export function Footer() {
  const [version, setVersion] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    fetch('/api/version')
      .then((r) => (r.ok ? r.json() : null))
      .then((d) => { if (!cancelled && d?.version) setVersion(d.version) })
      .catch(() => { /* ignore — show without version */ })
    return () => { cancelled = true }
  }, [])

  return (
    <footer className="app-footer">
      <div className="app-footer__row">
        <div className="app-footer__left">
          <span className="app-footer__brand">Trinity Tracker</span>
          {version && <span className="app-footer__version" title="Build version">{version}</span>}
        </div>
        <nav className="app-footer__center" aria-label="Footer links">
          <a href={DISCORD_INVITE_URL} target="_blank" rel="noopener noreferrer">Discord</a>
          <span className="app-footer__sep" aria-hidden="true">·</span>
          <a href={GITHUB_REPO_URL} target="_blank" rel="noopener noreferrer">GitHub</a>
          <span className="app-footer__sep" aria-hidden="true">·</span>
          <Link to="/docs/credits">Credits</Link>
        </nav>
        <div className="app-footer__right">
          <span>Made by <a href="https://ernie.io" target="_blank" rel="noopener noreferrer">NilClass</a></span>
        </div>
      </div>
    </footer>
  )
}
