import { useMemo } from "react";
import { ColoredText } from "./ColoredText";
import { cleanQ3DisplayName, canonicalizeDisplayName } from "../utils";

interface DisplayNameEditorProps {
  value: string;
  onChange: (value: string) => void;
  mode: "claim" | "restyle";
  originalCanonical?: string;
  error?: string;
  onSubmit?: () => void;
}

export function DisplayNameEditor({
  value,
  onChange,
  mode,
  originalCanonical,
  error,
  onSubmit,
}: DisplayNameEditorProps) {
  const cleaned = useMemo(() => cleanQ3DisplayName(value), [value]);
  const canonical = useMemo(() => canonicalizeDisplayName(cleaned), [cleaned]);
  const byteLen = new TextEncoder().encode(cleaned).length;

  const canonicalMismatch =
    mode === "restyle" &&
    originalCanonical != null &&
    canonical !== originalCanonical;

  return (
    <div className="display-name-editor">
      <div className="display-name-input-row">
        <input
          type="text"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && onSubmit) {
              e.preventDefault();
              onSubmit();
            }
          }}
          placeholder="^1Your^7Name"
          className="display-name-input"
          maxLength={63}
        />
        <span className="display-name-bytes">{byteLen}/31</span>
      </div>
      {cleaned && (
        <div className="display-name-preview">
          <ColoredText text={cleaned} />
        </div>
      )}
      {canonicalMismatch && (
        <div className="display-name-hint">
          Cannot change your name, only restyle it
        </div>
      )}
      {error && <div className="display-name-error">{error}</div>}
      <div className="display-name-help">
        {[1, 2, 3, 4, 5, 6, 7].map((n) => (
          <span key={n} className="color-chip">
            <span
              className="color-chip-swatch"
              style={{ background: `var(--q3-color-${n})` }}
            />
            ^{n}
          </span>
        ))}
      </div>
    </div>
  );
}

export function useDisplayNameValidation(
  value: string,
  mode: "claim" | "restyle",
  originalCanonical?: string,
) {
  const cleaned = useMemo(() => cleanQ3DisplayName(value), [value]);
  const canonical = useMemo(() => canonicalizeDisplayName(cleaned), [cleaned]);
  const canonicalMismatch =
    mode === "restyle" &&
    originalCanonical != null &&
    canonical !== originalCanonical;
  const isEmpty = cleaned === "" || canonical === "";
  return { cleaned, canonical, canonicalMismatch, isEmpty };
}
