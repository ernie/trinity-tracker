import { type ReactNode } from "react";
import { type Platform } from "./platformStorage";
import { usePlatform } from "./PlatformContext";

interface PlatformOnlyProps {
  platform: Platform | Platform[];
  children: ReactNode;
}

// Shows children only on the listed platforms, with no styling of its
// own; PlatformNote is the styled aside.
export function PlatformOnly({ platform, children }: PlatformOnlyProps) {
  const { platform: active } = usePlatform();
  const platforms = Array.isArray(platform) ? platform : [platform];
  if (!platforms.includes(active)) return null;
  return <>{children}</>;
}
