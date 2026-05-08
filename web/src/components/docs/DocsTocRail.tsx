import { NavLink, useLocation } from 'react-router-dom'

export interface DocSection {
  id: string
  label: string
}

export const DOCS_TABS = [
  { path: 'getting-started', label: 'Getting Started' },
  { path: 'features', label: 'Features' },
  { path: 'server-admin', label: 'Server Admin' },
  { path: 'credits', label: 'Credits' },
] as const

// Left rail: pure tab list across the four docs pages. The "where on
// this page" sub-section navigation lives exclusively in DocsOnThisPage
// (right rail) so the two rails have a clean division of responsibility:
// left = which page, right = where within this page.
export function DocsTocRail() {
  const location = useLocation()

  return (
    <nav className="docs-toc-rail" aria-label="Documentation sections">
      <div className="docs-toc-rail__title">Docs</div>
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
    </nav>
  )
}
