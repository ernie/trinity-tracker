import { useScrollSpy } from '../../hooks/useScrollSpy'
import type { DocSection } from './DocsTocRail'

interface DocsOnThisPageProps {
  sections: DocSection[]
}

// Right rail: scroll-spy-highlighted list of the current page's h2s.
// The tri-point glyph marks the active section and scales as you read.
export function DocsOnThisPage({ sections }: DocsOnThisPageProps) {
  const ids = sections.map((s) => s.id)
  const activeId = useScrollSpy(ids, 120)

  if (sections.length === 0) return null

  return (
    <nav className="docs-on-this-page" aria-label="On this page">
      <div className="docs-on-this-page__title">On this page</div>
      <ul className="docs-on-this-page__list">
        {sections.map((s) => (
          <li
            key={s.id}
            className={`docs-on-this-page__item ${activeId === s.id ? 'docs-on-this-page__item--active' : ''}`}
          >
            <a href={`#${s.id}`} className="docs-on-this-page__link">
              <img
                src="/assets/icon-128.png"
                alt=""
                aria-hidden="true"
                className="docs-on-this-page__glyph"
              />
              <span>{s.label}</span>
            </a>
          </li>
        ))}
      </ul>
    </nav>
  )
}
