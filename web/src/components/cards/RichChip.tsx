// Compound chip used as the top-left identifier on Server / Match cards:
//   [source · server · mode]
// Source is dimmed; server is accent; mode is ember default.

interface RichChipProps {
  source?: string
  server: string
  mode?: string  // e.g. "CPM · 1V1" or "VQ3 · TDM"
  className?: string
}

export function RichChip({ source, server, mode, className }: RichChipProps) {
  return (
    <span className={`rich-chip ${className ?? ''}`}>
      {source && (
        <>
          <span className="rich-chip__source">{source}</span>
          <span className="rich-chip__sep">·</span>
        </>
      )}
      <span className="rich-chip__server">{server}</span>
      {mode && (
        <>
          <span className="rich-chip__sep">·</span>
          <span className="rich-chip__mode">{mode}</span>
        </>
      )}
    </span>
  )
}
