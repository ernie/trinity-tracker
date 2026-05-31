import { type ReactNode } from "react";
import {
  usePlatform,
  PLATFORM_LABELS,
  PLATFORM_ORDER,
} from "./PlatformContext";
import { type Platform } from "./platformStorage";

interface PlatformTabsProps {
  children: ReactNode;
  // Some `<PlatformTabs>` blocks won't have a panel for every
  // platform (e.g., a VR-only feature with PCVR + Quest panels but
  // no Flatscreen). The tabs only render entries for the platforms
  // a Panel exists for.
}

// Container that picks the active panel from its <PlatformTabs.Panel>
// children based on the global PlatformContext. Clicking a tab flips
// global state, so every other PlatformTabs on the page syncs.
export function PlatformTabs({ children }: PlatformTabsProps) {
  const { platform, setPlatform } = usePlatform();

  // Walk children to find the set of platforms with panels and the
  // active panel. Children must be PlatformTabs.Panel elements; we
  // identify them by their displayName at runtime (no React internals
  // poking).
  const panels = new Map<Platform, ReactNode>();
  const childArray = Array.isArray(children) ? children : [children];
  for (const child of childArray) {
    if (
      child &&
      typeof child === "object" &&
      "props" in child &&
      "type" in child &&
      typeof child.type !== "string" &&
      "displayName" in child.type &&
      child.type.displayName === "PlatformTabs.Panel"
    ) {
      const p = (child.props as PanelProps).platform;
      panels.set(p, (child.props as PanelProps).children);
    }
  }

  const orderedPlatforms = PLATFORM_ORDER.filter((p) => panels.has(p));
  if (orderedPlatforms.length === 0) return null;

  // If the active platform has no panel here, fall back to the first
  // available — keeps the UI sensible when a content author only
  // authored two of three variants.
  const activePlatform = panels.has(platform) ? platform : orderedPlatforms[0];

  return (
    <div className="docs-platform-tabs">
      <div className="docs-platform-tabs__strip" role="tablist">
        {orderedPlatforms.map((p) => (
          <button
            key={p}
            type="button"
            role="tab"
            aria-selected={p === activePlatform}
            className={`docs-platform-tabs__tab ${p === activePlatform ? "is-active" : ""}`}
            onClick={() => setPlatform(p)}
          >
            {PLATFORM_LABELS[p]}
          </button>
        ))}
      </div>
      <div className="docs-platform-tabs__panel" role="tabpanel">
        {panels.get(activePlatform)}
      </div>
    </div>
  );
}

interface PanelProps {
  platform: Platform;
  children: ReactNode;
}

// Subcomponent for the per-platform content. Has displayName so the
// parent's child-walking can identify it without poking React
// internals.
function Panel({ children }: PanelProps) {
  return <>{children}</>;
}
Panel.displayName = "PlatformTabs.Panel";

PlatformTabs.Panel = Panel;
