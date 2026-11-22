import { useSyncExternalStore } from "react";
import {
  isHonkSoundOn,
  setHonkSoundOn,
  subscribeHonkSound,
} from "../utils/honkSound";

export function HonkSoundToggle() {
  const on = useSyncExternalStore(subscribeHonkSound, isHonkSoundOn);
  const label = on ? "Turn honk sounds off" : "Turn honk sounds on";

  return (
    <button
      type="button"
      className={`honk-toggle${on ? "" : " is-off"}`}
      onClick={() => setHonkSoundOn(!on)}
      aria-pressed={!on}
      aria-label={label}
      title={label}
    >
      <span className="honk-toggle__face">
        <img
          className="honk-toggle__goose"
          src="/assets/portraits/goose/icon_default.png"
          alt=""
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
