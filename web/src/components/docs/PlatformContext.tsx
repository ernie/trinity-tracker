import { createContext, useContext, useState, type ReactNode } from "react";
import { loadPlatform, savePlatform, type Platform } from "./platformStorage";

// Context value is null while the user hasn't picked yet — the
// PlatformProvider's children render the picker in that state, so any
// component inside the provider tree that calls usePlatform() can
// rely on a non-null platform. The setter accepts a non-null value
// only — there's no UI affordance to return to null after the first
// pick (per spec).
interface PlatformContextValue {
  platform: Platform;
  setPlatform: (p: Platform) => void;
}

const PlatformContext = createContext<PlatformContextValue | null>(null);

interface PlatformProviderProps {
  children: ReactNode;
  // Rendered when state is null (first visit). The provider passes a
  // pickHandler the picker calls when the user selects a platform.
  renderPicker: (onPick: (p: Platform) => void) => ReactNode;
}

// Provider hydrates state from localStorage during the initial render
// via useState's lazy initializer, persists every change, and gates
// its children behind the picker until a platform is chosen. Consumer
// components see only non-null platforms via usePlatform().
export function PlatformProvider({
  children,
  renderPicker,
}: PlatformProviderProps) {
  // Lazy initializer reads localStorage synchronously during mount,
  // so the first committed frame already reflects the stored choice —
  // no picker flicker, no extra effect-driven render.
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

// Hook for components that need the current platform. Throws if
// called outside a PlatformProvider — that's a programming bug and
// we want it loud.
export function usePlatform(): PlatformContextValue {
  const ctx = useContext(PlatformContext);
  if (!ctx) {
    throw new Error("usePlatform must be used inside <PlatformProvider>");
  }
  return ctx;
}

// Human-readable platform labels for UI. Centralized so picker, tabs,
// badge, and note all stay in lockstep.
export const PLATFORM_LABELS: Record<Platform, string> = {
  flatscreen: "Flatscreen",
  pcvr: "PCVR",
  quest: "Quest",
};

export const PLATFORM_DESCRIPTIONS: Record<Platform, string> = {
  flatscreen: "Trinity Engine on desktop monitor — keyboard + mouse.",
  pcvr: "Trinity VR on a PC-tethered VR headset.",
  quest: "Trinity Quest standalone on Meta Quest 2, 3, or 3S.",
};

export const PLATFORM_ORDER: readonly Platform[] = [
  "flatscreen",
  "pcvr",
  "quest",
] as const;
