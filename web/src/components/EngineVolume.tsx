import { useEffect, useRef, useState } from "react";
import type { EngineModule } from "../types";

export function EngineVolume({
  moduleRef,
  engineReady,
}: {
  moduleRef: React.MutableRefObject<EngineModule | null>;
  engineReady: boolean;
}) {
  const [sfxVolume, setSfxVolume] = useState(() => {
    const saved = localStorage.getItem("demo_sfx_volume");
    if (saved !== null) return parseFloat(saved);
    const old = localStorage.getItem("demo_volume");
    return old !== null ? parseFloat(old) : 0.5;
  });
  const [musicVolume, setMusicVolume] = useState(() => {
    const saved = localStorage.getItem("demo_music_volume");
    if (saved !== null) return parseFloat(saved);
    const old = localStorage.getItem("demo_volume");
    return old !== null ? parseFloat(old) : 0.5;
  });
  const [voipVolume, setVoipVolume] = useState(() => {
    const saved = localStorage.getItem("demo_voip_volume");
    if (saved !== null) {
      const v = parseFloat(saved);
      if (!Number.isNaN(v)) return v;
    }
    return 1.0;
  });
  const [muted, setMuted] = useState(
    () => localStorage.getItem("demo_muted") === "true",
  );
  const [volumeOpen, setVolumeOpen] = useState(false);
  const volumeWrapRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const mod = moduleRef.current;
    if (!engineReady || !mod?.ccall) return;
    const sv = muted ? 0 : sfxVolume;
    const mv = muted ? 0 : musicVolume;
    const vv = muted ? 0 : voipVolume;
    mod.ccall(
      "Cbuf_AddText",
      null,
      ["string"],
      [`s_volume ${sv}\ns_musicvolume ${mv}\ncl_voipVolume ${vv}\n`],
    );
  }, [sfxVolume, musicVolume, voipVolume, muted, engineReady, moduleRef]);

  useEffect(() => {
    localStorage.setItem("demo_sfx_volume", String(sfxVolume));
  }, [sfxVolume]);
  useEffect(() => {
    localStorage.setItem("demo_music_volume", String(musicVolume));
  }, [musicVolume]);
  useEffect(() => {
    localStorage.setItem("demo_voip_volume", String(voipVolume));
  }, [voipVolume]);
  useEffect(() => {
    localStorage.setItem("demo_muted", String(muted));
  }, [muted]);

  useEffect(() => {
    if (!volumeOpen) return;
    const handler = (e: TouchEvent) => {
      if (
        volumeWrapRef.current &&
        !volumeWrapRef.current.contains(e.target as Node)
      ) {
        setVolumeOpen(false);
      }
    };
    document.addEventListener("touchstart", handler);
    return () => document.removeEventListener("touchstart", handler);
  }, [volumeOpen]);

  return (
    <div className="ctrl-group">
      <div
        ref={volumeWrapRef}
        className={`ctrl-volume-wrap${volumeOpen ? " open" : ""}`}
      >
        <button
          className="ctrl-btn"
          title={muted ? "Unmute" : "Mute"}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => setMuted((m) => !m)}
          onTouchStart={(e) => {
            e.preventDefault();
            setVolumeOpen((prev) => !prev);
          }}
        >
          {(() => {
            const peakVol = Math.max(
              sfxVolume,
              musicVolume,
              Math.min(voipVolume, 1),
            );
            if (muted || peakVol === 0)
              return (
                <svg
                  viewBox="0 0 24 24"
                  width="18"
                  height="18"
                  fill="currentColor"
                >
                  <path d="M16.5 12c0-1.77-1.02-3.29-2.5-4.03v2.21l2.45 2.45c.03-.2.05-.41.05-.63zm2.5 0c0 .94-.2 1.82-.54 2.64l1.51 1.51C20.63 14.91 21 13.5 21 12c0-4.28-2.99-7.86-7-8.77v2.06c2.89.86 5 3.54 5 6.71zM4.27 3L3 4.27 7.73 9H3v6h4l5 5v-6.73l4.25 4.25c-.67.52-1.42.93-2.25 1.18v2.06c1.38-.31 2.63-.95 3.69-1.81L19.73 21 21 19.73l-9-9L4.27 3zM12 4L9.91 6.09 12 8.18V4z" />
                </svg>
              );
            if (peakVol < 0.5)
              return (
                <svg
                  viewBox="0 0 24 24"
                  width="18"
                  height="18"
                  fill="currentColor"
                >
                  <path d="M18.5 12c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02zM5 9v6h4l5 5V4L9 9H5z" />
                </svg>
              );
            return (
              <svg
                viewBox="0 0 24 24"
                width="18"
                height="18"
                fill="currentColor"
              >
                <path d="M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02zM14 3.23v2.06c2.89.86 5 3.54 5 6.71s-2.11 5.85-5 6.71v2.06c4.01-.91 7-4.49 7-8.77s-2.99-7.86-7-8.77z" />
              </svg>
            );
          })()}
        </button>
        <div className="ctrl-volume-panel">
          <div className="ctrl-volume-row">
            <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
              <path d="M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02zM14 3.23v2.06c2.89.86 5 3.54 5 6.71s-2.11 5.85-5 6.71v2.06c4.01-.91 7-4.49 7-8.77s-2.99-7.86-7-8.77z" />
            </svg>
            <input
              type="range"
              min={0}
              max={1}
              step={0.01}
              className="ctrl-volume-slider"
              value={muted ? 0 : sfxVolume}
              onChange={(e) => {
                const v = parseFloat(e.target.value);
                if (v > 0 && muted) setMuted(false);
                setSfxVolume(v);
              }}
              onMouseDown={(e) => e.stopPropagation()}
              onMouseUp={(e) => e.stopPropagation()}
              onTouchStart={(e) => e.stopPropagation()}
              onTouchEnd={(e) => e.stopPropagation()}
            />
          </div>
          <div className="ctrl-volume-row">
            <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
              <path d="M12 3v10.55c-.59-.34-1.27-.55-2-.55-2.21 0-4 1.79-4 4s1.79 4 4 4 4-1.79 4-4V7h4V3h-6z" />
            </svg>
            <input
              type="range"
              min={0}
              max={1}
              step={0.01}
              className="ctrl-volume-slider"
              value={muted ? 0 : musicVolume}
              onChange={(e) => {
                const v = parseFloat(e.target.value);
                if (v > 0 && muted) setMuted(false);
                setMusicVolume(v);
              }}
              onMouseDown={(e) => e.stopPropagation()}
              onMouseUp={(e) => e.stopPropagation()}
              onTouchStart={(e) => e.stopPropagation()}
              onTouchEnd={(e) => e.stopPropagation()}
            />
          </div>
          <div className="ctrl-volume-row">
            <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
              <path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3zm5.3-3c0 3-2.54 5.1-5.3 5.1S6.7 14 6.7 11H5c0 3.41 2.72 6.23 6 6.72V21h2v-3.28c3.28-.48 6-3.3 6-6.72h-1.7z" />
            </svg>
            <input
              type="range"
              min={0}
              max={2}
              step={0.01}
              className="ctrl-volume-slider"
              value={muted ? 0 : voipVolume}
              onChange={(e) => {
                const v = parseFloat(e.target.value);
                if (v > 0 && muted) setMuted(false);
                setVoipVolume(v);
              }}
              onMouseDown={(e) => e.stopPropagation()}
              onMouseUp={(e) => e.stopPropagation()}
              onTouchStart={(e) => e.stopPropagation()}
              onTouchEnd={(e) => e.stopPropagation()}
            />
          </div>
        </div>
      </div>
    </div>
  );
}
