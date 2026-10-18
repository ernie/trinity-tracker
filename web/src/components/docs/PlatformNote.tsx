import { type ReactNode } from "react";
import { type Platform } from "./platformStorage";
import { PLATFORM_LABELS, usePlatform } from "./PlatformContext";

interface PlatformNoteProps {
  platform: Platform | Platform[];
  children: ReactNode;
}

// A styled aside shown only on the listed platforms; its chip names
// them.
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
