import { NavLink, useLocation } from 'react-router-dom'
import { NavScroller } from '../NavScroller'

export interface DocSection {
  id: string
  label: string
}

// Credits moved out of the docs IA — now a top-level /credits route.
// Subsequent rework phases will replace these entries with the
// finalized 6-tab structure (Welcome / Install / Account / Customize
// / Server Admin / Reference). For now keep the existing tabs intact
// so the docs subroutes stay reachable while phase 2+ is in flight.
export const DOCS_TABS = [
  { path: 'getting-started', label: 'Getting Started' },
  { path: 'features', label: 'Features' },
  { path: 'server-admin', label: 'Server Admin' },
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
            const isActive = location.pathname.startsWith(`/docs/${tab.path}`)
            return (
              <li key={tab.path} className={`docs-toc-rail__item ${isActive ? 'docs-toc-rail__item--active' : ''}`}>
                <NavLink to={`/docs/${tab.path}`} className="docs-toc-rail__tab">
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
