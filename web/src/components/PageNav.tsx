import { useState, useRef, useEffect } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useGitHubReleases } from '../hooks/useGitHubReleases'

const NAV_ITEMS = [
  { path: '/', label: 'Servers' },
  { path: '/players', label: 'Players' },
  { path: '/matches', label: 'Matches' },
  { path: '/leaderboard', label: 'Leaderboard' },
  { path: '/docs', label: 'Docs' },
]

export function PageNav() {
  const location = useLocation()
  const { releases } = useGitHubReleases()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  const isActive = (path: string) => {
    if (path === '/') return location.pathname === '/'
    if (path === '/docs') return location.pathname.startsWith('/docs')
    return location.pathname.startsWith(path)
  }

  useEffect(() => {
    if (!open) return
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  // Close on navigation
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { setOpen(false) }, [location.pathname])

  return (
    <nav className="page-nav">
      {NAV_ITEMS.map(item => (
        <Link
          key={item.path}
          to={item.path}
          className={`nav-link ${isActive(item.path) ? 'active' : ''}`}
        >
          {item.label}
        </Link>
      ))}
      <div className="download-dropdown" ref={ref}>
        <button
          className="download-toggle"
          onClick={() => setOpen(o => !o)}
          title="Get Trinity"
          aria-label="Get Trinity"
        >
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
            <path d="M8 2v8M5 7l3 3 3-3M3 12h10" />
          </svg>
        </button>
        {open && (
          <div className="download-panel">
            <div className="download-panel-title">Get Trinity</div>
            {releases.map(r => (
              <a
                key={r.repo}
                href={r.url}
                target="_blank"
                rel="noopener noreferrer"
                className="download-item"
              >
                <span className="download-item-name">
                  {r.displayName}
                  {r.bundled && (
                    <span
                      className="download-item-bundled"
                      onClick={e => e.preventDefault()}
                    >
                      <img src="/assets/icon-128.png" alt="" className="download-item-bundled-icon" />
                      <span className="download-item-bundled-tip">Includes Trinity mod</span>
                    </span>
                  )}
                </span>
                {r.version && <span className="download-item-version">{r.version}</span>}
              </a>
            ))}
            <Link to="/quake3-eula" className="download-item download-item-secondary">
              <span className="download-item-name">Quake 3 1.32 patches</span>
              <span className="download-item-version">EULA</span>
            </Link>
            <Link to="/docs/getting-started" className="getting-started-cta getting-started-cta-sm">
              Get started with Trinity
            </Link>
          </div>
        )}
      </div>
    </nav>
  )
}
