// Three platform identities, one stored at a time. `null` = "user has
// never picked"; surfaced by the docs only as the gate for the
// first-visit picker. After picking, the stored value is always one
// of the three concrete platforms.
export type Platform = "flatscreen" | "pcvr" | "quest";

export const PLATFORM_STORAGE_KEY = "q3a_docs_platform";

const VALID: ReadonlySet<string> = new Set(["flatscreen", "pcvr", "quest"]);

// Defensive read — anything not in the valid set (including a stale
// value left by a future-self typo) returns null and triggers the
// re-pick flow rather than silently rendering wrong-platform content.
export function loadPlatform(): Platform | null {
  try {
    const raw = localStorage.getItem(PLATFORM_STORAGE_KEY);
    if (raw && VALID.has(raw)) return raw as Platform;
    return null;
  } catch {
    return null;
  }
}

export function savePlatform(p: Platform): void {
  try {
    localStorage.setItem(PLATFORM_STORAGE_KEY, p);
  } catch {
    // Private-browsing throws on setItem; nothing to do, the picker
    // will just re-fire next visit.
  }
}
