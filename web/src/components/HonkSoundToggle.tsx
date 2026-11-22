import { useState, useSyncExternalStore } from "react";
import {
  isHonkSoundOn,
  setHonkSoundOn,
  subscribeHonkSound,
} from "../utils/honkSound";

// Heads don't honk under reduced motion, so the switch would do nothing.
const reducedMotion =
  typeof window !== "undefined" &&
  !!window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;

export function HonkSoundToggle() {
  const on = useSyncExternalStore(subscribeHonkSound, isHonkSoundOn);
  const [missing, setMissing] = useState(false);
  if (reducedMotion || missing) return null;

  return (
    <button
      type="button"
      className={`honk-toggle${on ? "" : " is-off"}`}
      onClick={() => setHonkSoundOn(!on)}
      aria-pressed={on}
      aria-label="Honk sounds"
      title={on ? "Honk sounds on" : "Honk sounds off"}
    >
      <span className="honk-toggle__face">
        <img
          className="honk-toggle__goose"
          src="/assets/portraits/goose/icon_default.png"
          alt=""
          onError={() => setMissing(true)}
        />
      </span>
      {on && (
        <img
          className="honk-toggle__honk"
          src="/assets/heads/goose/honk_2.png"
          alt=""
        />
      )}
      {!on && (
        <svg className="honk-toggle__no" viewBox="0 0 32 32" aria-hidden="true">
          <circle cx="16" cy="16" r="14.3" />
          <path d="M6.8 6.8l18.4 18.4" />
        </svg>
      )}
    </button>
  );
}
