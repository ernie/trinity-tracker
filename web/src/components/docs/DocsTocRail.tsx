import { useEffect, useRef } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { NavScroller } from '../NavScroller'

export interface DocSection {
  id: string
  label: string
}

// Seven-tab docs IA: Welcome (the index) + six subroutes. The Welcome
// entry uses an empty path because it lives at /docs root; the
// active-state check below special-cases it. Play sits between
// Install and Account so the rail tracks the natural reading order:
// install Trinity → tour what it does → set up your account → tune
// to taste → operate a server (if applicable) → look up specifics.
export const DOCS_TABS = [
  { path: '', label: 'Welcome' },
  { path: 'install', label: 'Install' },
  { path: 'play', label: 'Play' },
  { path: 'account', label: 'Account' },
  { path: 'customize', label: 'Customize' },
  { path: 'admin', label: 'Server Admin' },
  { path: 'reference', label: 'Reference' },
] as const

// Left rail: pure tab list across the docs pages. NavScroller wraps
// the list so it picks up the chevron + fade-mask affordances when
// the buttons overflow on narrow viewports — same pattern as the
// page-nav and admin-sidebar nav strips.
export function DocsTocRail() {
  const location = useLocation()
  const navRef = useRef<HTMLElement>(null)

  // On the mobile horizontal layout the rail can be wider than the
  // viewport, and a direct link (e.g. landing on /docs/reference)
  // leaves the active tab off-screen. Scroll the active item into
  // view whenever the route changes. `block: 'nearest'` keeps the
  // page's vertical scroll undisturbed; `inline: 'center'` centers
  // the active tab horizontally inside its scroll container. On
  // desktop the rail isn't horizontally scrollable, so this is a
  // no-op there.
  useEffect(() => {
    const active = navRef.current?.querySelector('.docs-toc-rail__item--active')
    if (active instanceof HTMLElement) {
      active.scrollIntoView({ inline: 'center', block: 'nearest', behavior: 'smooth' })
    }
  }, [location.pathname])

  return (
    <nav ref={navRef} className="docs-toc-rail" aria-label="Documentation sections">
      <div className="docs-toc-rail__title">Docs</div>
      <NavScroller scrollClassName="docs-toc-rail__scroll">
        <ul className="docs-toc-rail__list">
          {DOCS_TABS.map((tab) => {
            const isActive = tab.path === ''
              ? location.pathname === '/docs' || location.pathname === '/docs/'
              : location.pathname.startsWith(`/docs/${tab.path}`)
            return (
              <li key={tab.path} className={`docs-toc-rail__item ${isActive ? 'docs-toc-rail__item--active' : ''}`}>
                <NavLink to={tab.path === '' ? '/docs' : `/docs/${tab.path}`} className="docs-toc-rail__tab">
                  {tab.label}
                </NavLink>
              </li>
            )
          })}
        </ul>
      </NavScroller>
    </nav>
  )
}
