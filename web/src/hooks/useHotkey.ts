import { useEffect } from "react";

export interface HotkeySpec {
  // Lowercase key name (e.g. 'k', '/', 'escape').
  key: string;
  // True if the modifier (Cmd on Mac, Ctrl elsewhere) is required.
  meta?: boolean;
}

// Document-level shortcut. It skips keys typed into an editable element,
// except Escape, which always fires.
export function useHotkey(
  spec: HotkeySpec,
  handler: (e: KeyboardEvent) => void,
) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      // Synthetic keydowns (browser/password-manager autofill) can
      // arrive without a key.
      const key = e.key?.toLowerCase();
      if (!key || key !== spec.key) return;
      if (spec.meta) {
        const meta = e.metaKey || e.ctrlKey;
        if (!meta) return;
      }
      if (key !== "escape") {
        const target = e.target as HTMLElement | null;
        if (
          target &&
          (target.tagName === "INPUT" ||
            target.tagName === "TEXTAREA" ||
            target.isContentEditable)
        )
          return;
      }
      e.preventDefault();
      handler(e);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [spec.key, spec.meta, handler]);
}
