import {
  usePlatform,
  PLATFORM_LABELS,
  PLATFORM_ORDER,
} from "./PlatformContext";

// Persistent rail-mounted indicator + flipper. Shows the current
// platform and lets the user change it without hunting for an inline
// PlatformTabs. Mirrors the same context — flipping here syncs all
// inline tabs on the page.
export function PlatformBadge() {
  const { platform, setPlatform } = usePlatform();

  return (
    <div className="docs-platform-badge" role="group" aria-label="Platform">
      <span className="docs-platform-badge__label">Platform</span>
      <div className="docs-platform-badge__strip" role="tablist">
        {PLATFORM_ORDER.map((p) => (
          <button
            key={p}
            type="button"
            role="tab"
            aria-selected={p === platform}
            className={`docs-platform-badge__btn ${p === platform ? "is-active" : ""}`}
            onClick={() => setPlatform(p)}
            title={PLATFORM_LABELS[p]}
          >
            {PLATFORM_LABELS[p]}
          </button>
        ))}
      </div>
    </div>
  );
}
