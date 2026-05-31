import { useEffect, useRef } from "react";
import { PLATFORM_LABELS } from "./PlatformContext";
import type { Platform } from "./platformStorage";

interface ReferenceSearchProps {
  value: string;
  onChange: (value: string) => void;
  showAllPlatforms: boolean;
  onToggleShowAll: () => void;
  currentPlatform: Platform;
}

// Sticky search input + platform-filter toggle pinned to the top of
// the Reference content area. `/` keybinding focuses the search input
// (skipped when the user is already typing in a form field). The
// toggle flips between "filter to current platform" (default) and
// "show all platforms" — its label always describes the *action* the
// click will perform, not the current state.
export function ReferenceSearch({
  value,
  onChange,
  showAllPlatforms,
  onToggleShowAll,
  currentPlatform,
}: ReferenceSearchProps) {
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      // Skip when the user is already typing somewhere editable, when
      // any modifier key is held, or when the input is already focused.
      if (e.key !== "/" || e.ctrlKey || e.metaKey || e.altKey) return;
      const target = e.target as HTMLElement | null;
      if (target) {
        const tag = target.tagName.toLowerCase();
        if (tag === "input" || tag === "textarea" || tag === "select") return;
        if (target.isContentEditable) return;
      }
      e.preventDefault();
      inputRef.current?.focus();
      inputRef.current?.select();
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, []);

  const platformLabel = PLATFORM_LABELS[currentPlatform];

  return (
    <div className="docs-reference-search">
      <input
        ref={inputRef}
        type="search"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder="Filter cvars and glossary…  ( / to focus )"
        aria-label="Filter cvars and glossary"
      />
      {value && (
        <button
          type="button"
          className="docs-reference-search__clear"
          onClick={() => {
            onChange("");
            inputRef.current?.focus();
          }}
          aria-label="Clear filter"
        >
          ×
        </button>
      )}
      <button
        type="button"
        className="docs-reference-search__platform-toggle"
        onClick={onToggleShowAll}
        title={
          showAllPlatforms
            ? `Filter cvars to ${platformLabel} only`
            : "Show cvars for all platforms, not just this one"
        }
      >
        {showAllPlatforms ? `Filter to ${platformLabel}` : "Show all platforms"}
      </button>
    </div>
  );
}
