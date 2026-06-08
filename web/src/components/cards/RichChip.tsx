import type { ReactNode } from "react";

// Compound chip used as the top-left identifier on Server / Match cards:
//   [extras · source · server · mode]
// `children` (e.g. mode-icon thumbnails) leads so the icon glyphs anchor
// the chip visually. Source is dimmed; server is accent; mode is ember
// default.

interface RichChipProps {
  source?: string;
  server: string;
  mode?: string; // e.g. "CPM · 1V1" or "VQ3 · TDM"
  className?: string;
  children?: ReactNode;
}

export function RichChip({
  source,
  server,
  mode,
  className,
  children,
}: RichChipProps) {
  return (
    <span
      className={`rich-chip ${className ?? ""}`}
      data-help={`Server identity:
• Mode icon — Quake 3, CPMA, Quake Live, or Quake Live Turbo
• Source — the host running this Q3 server. One host can serve multiple game servers.
• Server key — short identifier
• Game type — match format`}
    >
      {children && (
        <>
          <span className="rich-chip__extras">{children}</span>
          <span className="rich-chip__sep">·</span>
        </>
      )}
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
  );
}
