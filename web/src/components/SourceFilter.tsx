import { useSources } from '../hooks/useSources'

interface SourceFilterProps {
  value: string                       // empty string means "all sources"
  onChange: (next: string) => void
  className?: string
}

// Renders a <select> populated from /api/sources. Hidden when only one
// source exists. Inactive sources show "(inactive)" in the option label
// so users can still filter historical matches by them.
export function SourceFilter({ value, onChange, className }: SourceFilterProps) {
  const { sources, hasMultiple } = useSources()
  if (!hasMultiple) return null

  return (
    <select
      className={`source-filter${className ? ' ' + className : ''}`}
      value={value}
      onChange={e => onChange(e.target.value)}
      aria-label="Filter by source"
    >
      <option value="">All sources</option>
      {sources.map(s => (
        <option key={s.source} value={s.source}>
          {s.source}{s.active ? '' : ' (inactive)'}
        </option>
      ))}
    </select>
  )
}
