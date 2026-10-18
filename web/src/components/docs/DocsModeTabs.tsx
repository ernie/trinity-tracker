import { useState, type ReactNode } from "react";

type Mode = "baseq3" | "teamarena";

interface DocsModeTabsProps {
  initial?: Mode;
  children: ReactNode;
}

// Each instance keeps its own state: switching between baseq3 and Team
// Arena content is a per-section choice, where the platform is a
// choice about the reader's client. Children must be
// <DocsModeTabs.Panel mode="…">, found the same way as in PlatformTabs.
export function DocsModeTabs({
  initial = "baseq3",
  children,
}: DocsModeTabsProps) {
  const [mode, setMode] = useState<Mode>(initial);

  const panels = new Map<Mode, ReactNode>();
  const childArray = Array.isArray(children) ? children : [children];
  for (const child of childArray) {
    if (
      child &&
      typeof child === "object" &&
      "props" in child &&
      "type" in child &&
      typeof child.type !== "string" &&
      "displayName" in child.type &&
      child.type.displayName === "DocsModeTabs.Panel"
    ) {
      const m = (child.props as PanelProps).mode;
      panels.set(m, (child.props as PanelProps).children);
    }
  }

  const ORDER: Mode[] = ["baseq3", "teamarena"];
  const ordered = ORDER.filter((m) => panels.has(m));
  if (ordered.length === 0) return null;
  const active = panels.has(mode) ? mode : ordered[0];

  return (
    <div className="docs-mode-tabs">
      <div className="docs-mode-tabs__strip" role="tablist">
        {ordered.map((m) => (
          <button
            key={m}
            type="button"
            role="tab"
            aria-selected={m === active}
            className={`docs-mode-tabs__tab ${m === active ? "is-active" : ""}`}
            onClick={() => setMode(m)}
          >
            {MODE_LABELS[m]}
          </button>
        ))}
      </div>
      <div className="docs-mode-tabs__panel" role="tabpanel">
        {panels.get(active)}
      </div>
    </div>
  );
}

interface PanelProps {
  mode: Mode;
  children: ReactNode;
}

function Panel({ children }: PanelProps) {
  return <>{children}</>;
}
Panel.displayName = "DocsModeTabs.Panel";

DocsModeTabs.Panel = Panel;

const MODE_LABELS: Record<Mode, string> = {
  baseq3: "Quake III",
  teamarena: "Team Arena",
};
