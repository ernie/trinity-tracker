import { NavLink, useLocation } from 'react-router-dom'
import { NavScroller } from '../NavScroller'

export interface DocSection {
  id: string
  label: string
}

// Six-tab docs IA: Welcome (the index) + five subroutes. The Welcome
// entry uses an empty path because it lives at /docs root; the
// active-state check below special-cases it.
export const DOCS_TABS = [
  { path: '', label: 'Welcome' },
  { path: 'install', label: 'Install' },
  { path: 'account', label: 'Account' },
  { path: 'customize', label: 'Customize' },
  { path: 'server-admin', label: 'Server Admin' },
  { path: 'reference', label: 'Reference' },
] as const

// Left rail: pure tab list across the docs pages. NavScroller wraps
// the list so it picks up the chevron + fade-mask affordances when
// the buttons overflow on narrow viewports — same pattern as the
// page-nav and admin-sidebar nav strips.
export function DocsTocRail() {
  const location = useLocation()

  return (
    <nav className="docs-toc-rail" aria-label="Documentation sections">
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
