import { type ReactNode } from "react";
import {
  usePlatform,
  PLATFORM_LABELS,
  PLATFORM_ORDER,
} from "./PlatformContext";
import { type Platform } from "./platformStorage";

interface PlatformTabsProps {
  children: ReactNode;
}

// A tab sets the global platform, so every PlatformTabs on the page
// switches together. Tabs appear only for platforms that have a panel.
export function PlatformTabs({ children }: PlatformTabsProps) {
  const { platform, setPlatform } = usePlatform();

  // Panels are recognized by displayName.
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

function Panel({ children }: PanelProps) {
  return <>{children}</>;
}
Panel.displayName = "PlatformTabs.Panel";

PlatformTabs.Panel = Panel;
