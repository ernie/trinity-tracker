import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import {
  isHonkSoundOn,
  setHonkSoundOn,
  subscribeHonkSound,
} from "../utils/honkSound";

// Heads don't honk under reduced motion, so the switch would do nothing.
const reducedMotion =
  typeof window !== "undefined" &&
  !!window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;

const TAP_NOTE_MS = 4000;

export function HonkSoundToggle() {
  const on = useSyncExternalStore(subscribeHonkSound, isHonkSoundOn);
  const [missing, setMissing] = useState(false);
  const [noteOpen, setNoteOpen] = useState(false);
  const wrapRef = useRef<HTMLSpanElement>(null);
  const pointerRef = useRef("mouse");
  const timerRef = useRef(0);

  // A tap shows the note for a while; a tap elsewhere closes it.
  useEffect(() => {
    if (!noteOpen) return;
    const close = (e: PointerEvent) => {
      if (!wrapRef.current?.contains(e.target as Node)) setNoteOpen(false);
    };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, [noteOpen]);
  useEffect(() => () => window.clearTimeout(timerRef.current), []);

  if (reducedMotion || missing) return null;

  const showForTap = () => {
    setNoteOpen(true);
    window.clearTimeout(timerRef.current);
    timerRef.current = window.setTimeout(() => setNoteOpen(false), TAP_NOTE_MS);
  };

  return (
    <span
      ref={wrapRef}
      className="honk-toggle-wrap"
      onPointerEnter={(e) => {
        if (e.pointerType === "mouse") setNoteOpen(true);
      }}
      onPointerLeave={(e) => {
        if (e.pointerType === "mouse") setNoteOpen(false);
      }}
      onFocus={() => setNoteOpen(true)}
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null))
          setNoteOpen(false);
      }}
    >
      <button
        type="button"
        className={`honk-toggle${on ? "" : " is-off"}`}
        onPointerDown={(e) => {
          pointerRef.current = e.pointerType;
        }}
        onClick={() => {
          setHonkSoundOn(!on);
          if (pointerRef.current !== "mouse") showForTap();
        }}
        aria-pressed={on}
        aria-label="Honk sounds"
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
          <svg
            className="honk-toggle__no"
            viewBox="0 0 32 32"
            aria-hidden="true"
          >
            <circle cx="16" cy="16" r="14.3" />
            <path d="M6.8 6.8l18.4 18.4" />
          </svg>
        )}
      </button>
      {noteOpen && (
        <span className="honk-note" role="note">
          <span className="honk-note__card">
            Achieve total goose embodiment.{" "}
            <a
              href="https://gander.ernie.io"
              target="_blank"
              rel="noopener noreferrer"
            >
              Gander
            </a>{" "}
            is a free VR mod for Untitled Goose Game.
          </span>
        </span>
      )}
    </span>
  );
}
