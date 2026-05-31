import { type ReactNode } from "react";
import { type Platform } from "./platformStorage";
import { PLATFORM_LABELS, usePlatform } from "./PlatformContext";

interface PlatformNoteProps {
  platform: Platform | Platform[];
  children: ReactNode;
}

// Inline callout for a small platform-specific aside. Renders only
// when the active platform is in the target list; the chip in the
// header surfaces which platform(s) the note targets so a reader who
// switches platforms knows what they're seeing. Use when the
// deviation is small enough that a full PlatformTabs block would be
// overkill (e.g., a single asset path or gotcha).
export function PlatformNote({ platform, children }: PlatformNoteProps) {
  const { platform: active } = usePlatform();
  const platforms = Array.isArray(platform) ? platform : [platform];
  if (!platforms.includes(active)) return null;

  const label = platforms.map((p) => PLATFORM_LABELS[p]).join(" · ");

  return (
    <aside className="docs-platform-note">
      <div className="docs-platform-note__header">
        <span className="docs-platform-note__chip">{label}</span>
      </div>
      <div className="docs-platform-note__body">{children}</div>
    </aside>
  );
}
