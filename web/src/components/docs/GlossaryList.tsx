import type { GlossaryEntry } from '../../data/glossary'

interface GlossaryListProps {
  entries: GlossaryEntry[]
}

// Glossary terms rendered as a definition list-style card list.
// Receives an already-filtered list — filtering happens in the parent.
export function GlossaryList({ entries }: GlossaryListProps) {
  if (entries.length === 0) {
    return <p className="docs-reference__empty">No matches.</p>
  }
  return (
    <ul className="glossary-list">
      {entries.map((entry) => (
        <li key={entry.term} className="glossary-entry">
          <strong className="glossary-entry__term">{entry.term}</strong>
          <p className="glossary-entry__definition">{entry.definition}</p>
        </li>
      ))}
    </ul>
  )
}
