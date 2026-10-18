import { createContext, useContext, useState, type ReactNode } from "react";
import { loadPlatform, savePlatform, type Platform } from "./platformStorage";

// The provider shows the picker until a platform is chosen, so
// usePlatform() always returns one.
interface PlatformContextValue {
  platform: Platform;
  setPlatform: (p: Platform) => void;
}

const PlatformContext = createContext<PlatformContextValue | null>(null);

interface PlatformProviderProps {
  children: ReactNode;
  // Rendered until a platform is chosen.
  renderPicker: (onPick: (p: Platform) => void) => ReactNode;
}

export function PlatformProvider({
  children,
  renderPicker,
}: PlatformProviderProps) {
  // Read during the first render so a stored choice never flashes the
  // picker.
  const [platform, setPlatformState] = useState<Platform | null>(loadPlatform);

  const setPlatform = (p: Platform) => {
    savePlatform(p);
    setPlatformState(p);
  };

  if (platform === null) {
    return <>{renderPicker(setPlatform)}</>;
  }

  return (
    <PlatformContext.Provider value={{ platform, setPlatform }}>
      {children}
    </PlatformContext.Provider>
  );
}

export function usePlatform(): PlatformContextValue {
  const ctx = useContext(PlatformContext);
  if (!ctx) {
    throw new Error("usePlatform must be used inside <PlatformProvider>");
  }
  return ctx;
}

export const PLATFORM_LABELS: Record<Platform, string> = {
  flatscreen: "Flatscreen",
  vr: "VR",
};

export const PLATFORM_DESCRIPTIONS: Record<Platform, string> = {
  flatscreen: "On a monitor with keyboard and mouse.",
  vr: "In a VR headset: SteamVR on a PC, Steam Frame, Meta Quest, or PICO.",
};

export const PLATFORM_ORDER: readonly Platform[] = [
  "flatscreen",
  "vr",
] as const;
