import { type Platform } from "./platformStorage";
import {
  PLATFORM_LABELS,
  PLATFORM_DESCRIPTIONS,
  PLATFORM_ORDER,
} from "./PlatformContext";

interface PlatformPickerProps {
  onPick: (p: Platform) => void;
}

// First-visit blocking picker. Mounted by PlatformProvider when
// localStorage has no stored platform. No "skip" or "decide later"
// affordance — picking is required to enter the docs.
export function PlatformPicker({ onPick }: PlatformPickerProps) {
  return (
    <div
      className="docs-platform-picker"
      role="dialog"
      aria-modal="true"
      aria-labelledby="platform-picker-title"
    >
      <div className="docs-platform-picker__card">
        <h1 id="platform-picker-title" className="docs-platform-picker__title">
          How do you play Trinity?
        </h1>
        <p className="docs-platform-picker__lead">
          Pick the way you play and we'll tailor the docs to your setup. You can
          change this any time from the docs sidebar.
        </p>
        <div className="docs-platform-picker__options">
          {PLATFORM_ORDER.map((p) => (
            <button
              key={p}
              type="button"
              className="docs-platform-picker__option"
              onClick={() => onPick(p)}
            >
              <span className="docs-platform-picker__option-label">
                {PLATFORM_LABELS[p]}
              </span>
              <span className="docs-platform-picker__option-desc">
                {PLATFORM_DESCRIPTIONS[p]}
              </span>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
