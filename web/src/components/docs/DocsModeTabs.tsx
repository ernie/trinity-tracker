import { useState, type ReactNode } from 'react'

type Mode = 'baseq3' | 'teamarena'

interface DocsModeTabsProps {
  // Optional initial mode for the tabs; defaults to baseq3.
  initial?: Mode
  children: ReactNode
}

// Local-state segmented control for switching between baseq3 (vanilla
// Quake 3) and Team Arena (missionpack) inline content. Visually echoes
// PlatformTabs but each instance tracks its own state — the divergence
// here is the active asset folder / game mod, not the user's client, so
// global persistence isn't appropriate (a baseq3 player browsing the
// docs hasn't "chosen" baseq3 in the same way they pick a platform).
//
// Children must be <DocsModeTabs.Panel mode="…">; the parent picks the
// active panel by displayName + mode prop, the same lightweight
// child-walking pattern PlatformTabs uses.
export function DocsModeTabs({ initial = 'baseq3', children }: DocsModeTabsProps) {
  const [mode, setMode] = useState<Mode>(initial)

  const panels = new Map<Mode, ReactNode>()
  const childArray = Array.isArray(children) ? children : [children]
  for (const child of childArray) {
    if (
      child &&
      typeof child === 'object' &&
      'props' in child &&
      'type' in child &&
      typeof child.type !== 'string' &&
      'displayName' in child.type &&
      child.type.displayName === 'DocsModeTabs.Panel'
    ) {
      const m = (child.props as PanelProps).mode
      panels.set(m, (child.props as PanelProps).children)
    }
  }

  const ORDER: Mode[] = ['baseq3', 'teamarena']
  const ordered = ORDER.filter((m) => panels.has(m))
  if (ordered.length === 0) return null
  const active = panels.has(mode) ? mode : ordered[0]

  return (
    <div className="docs-mode-tabs">
      <div className="docs-mode-tabs__strip" role="tablist">
        {ordered.map((m) => (
          <button
            key={m}
            type="button"
            role="tab"
            aria-selected={m === active}
            className={`docs-mode-tabs__tab ${m === active ? 'is-active' : ''}`}
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
  )
}

interface PanelProps {
  mode: Mode
  children: ReactNode
}

function Panel({ children }: PanelProps) {
  return <>{children}</>
}
Panel.displayName = 'DocsModeTabs.Panel'

DocsModeTabs.Panel = Panel

// User-facing labels prefer the recognizable game names ("Quake III",
// "Team Arena") over their asset-folder identifiers (baseq3,
// missionpack). The internal Mode keys keep the asset-folder names
// since those are the canonical engine-side identifiers that show up
// in pk3 paths and #ifdef MISSIONPACK guards — translation happens
// here, at the UI boundary.
const MODE_LABELS: Record<Mode, string> = {
  baseq3: 'Quake III',
  teamarena: 'Team Arena',
}
