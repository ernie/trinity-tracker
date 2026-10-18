export type Platform = "flatscreen" | "vr";

export const PLATFORM_STORAGE_KEY = "q3a_docs_platform";

const VALID: ReadonlySet<string> = new Set(["flatscreen", "vr"]);

// null means no valid choice is stored, so the picker shows.
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
    // Private browsing can throw here; the picker shows again next
    // visit.
  }
}
