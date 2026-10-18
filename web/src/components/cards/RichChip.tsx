import type { ReactNode } from "react";

// The card's identity chip: [extras · source · server · mode].

interface RichChipProps {
  source?: string;
  server: string;
  mode?: string;
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
• Mode icon: the server's ruleset (Quake III Arena, CPMA, Quake Live or Quake Live Turbo)
• Source: the host running this server. One host can run several game servers.
• Server key: the server's short name
• Game type: the match format`}
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
