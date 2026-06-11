import { useState, useEffect, useCallback } from "react";
import { usePortraitManifest } from "../hooks/usePortraits";
import { apiFetch } from "../authFetch";

interface PortraitPickerModalProps {
  /** Current explicit choice ("model/skin"), or null when following the
   *  in-game model. */
  current: string | null;
  onSaved: () => void;
  onClose: () => void;
}

export function PortraitPickerModal({
  current,
  onSaved,
  onClose,
}: PortraitPickerModalProps) {
  const { loaded, data } = usePortraitManifest();
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  const handleBackdropClick = useCallback(
    (e: React.MouseEvent) => {
      if (e.target === e.currentTarget) onClose();
    },
    [onClose],
  );

  const save = async (portrait: string | null) => {
    setSaving(true);
    setError("");
    try {
      const res = await apiFetch("/api/account/portrait", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ portrait }),
      });
      if (!res.ok) {
        const body = await res.json();
        setError(body.error || "Failed to update profile icon");
        return;
      }
      onSaved();
    } catch {
      setError("Network error");
    } finally {
      setSaving(false);
    }
  };

  const models = data
    .filter((entry) => entry.skins.includes("default"))
    .map((entry) => entry.model);

  return (
    <div className="modal-backdrop" onClick={handleBackdropClick}>
      <div
        className="portrait-picker-modal"
        role="dialog"
        aria-label="Choose profile icon"
      >
        <button
          onClick={onClose}
          className="portrait-picker-close"
          aria-label="Close"
        >
          &times;
        </button>
        <h2>Choose Profile Icon</h2>
        {error && <div className="error-message">{error}</div>}
        {current && (
          <button
            className="generate-btn portrait-picker-reset"
            disabled={saving}
            onClick={() => void save(null)}
          >
            Use last in-game appearance
          </button>
        )}
        {!loaded ? (
          <div className="loading">Loading...</div>
        ) : models.length === 0 ? (
          <p className="no-player">Portraits are not available.</p>
        ) : (
          <div className="portrait-picker-scroll">
            <div className="portrait-picker-grid">
              {models.map((model) => {
                const value = `${model}/default`;
                return (
                  <button
                    key={model}
                    className={`portrait-option${current === value ? " selected" : ""}`}
                    disabled={saving}
                    onClick={() => void save(value)}
                    title={model}
                  >
                    <img
                      src={`/assets/portraits/${model}/icon_default.png`}
                      alt={model}
                      loading="lazy"
                    />
                  </button>
                );
              })}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
