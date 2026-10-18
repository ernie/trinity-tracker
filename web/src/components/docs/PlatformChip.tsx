import { type Platform } from "./platformStorage";
import { PLATFORM_LABELS } from "./PlatformContext";

interface PlatformChipProps {
  platform: Platform | Platform[];
}

// Labels content as platform-specific; PlatformOnly does the filtering.
export function PlatformChip({ platform }: PlatformChipProps) {
  const platforms = Array.isArray(platform) ? platform : [platform];
  const label = platforms.map((p) => PLATFORM_LABELS[p]).join(" · ");
  return <span className="docs-platform-chip">{label}</span>;
}
