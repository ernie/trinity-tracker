import { useEffect, useRef, useState } from "react";
import { Header } from "./Header";

const PATCH_ZIP_URL = "/downloads/quake3-1.32-pk3s.zip";
const SCROLL_TOLERANCE_PX = 4;

export function Quake3EulaPage() {
  const [eula, setEula] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [scrolled, setScrolled] = useState(false);
  const [agreed, setAgreed] = useState(false);
  const boxRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let cancelled = false;
    fetch("/quake3-eula.txt")
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        return r.text();
      })
      .then((text) => {
        if (!cancelled) setEula(text);
      })
      .catch((e) => {
        if (!cancelled) setLoadError(e instanceof Error ? e.message : String(e));
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // Re-evaluate whether the box is already at bottom whenever content
  // arrives — short EULA + tall viewport means the user never scrolls,
  // and we'd otherwise leave the agree checkbox locked forever.
  useEffect(() => {
    if (!eula || !boxRef.current) return;
    const el = boxRef.current;
    if (el.scrollHeight - el.clientHeight <= SCROLL_TOLERANCE_PX) {
      setScrolled(true);
    }
  }, [eula]);

  function handleScroll() {
    const el = boxRef.current;
    if (!el) return;
    if (el.scrollTop + el.clientHeight >= el.scrollHeight - SCROLL_TOLERANCE_PX) {
      setScrolled(true);
    }
  }

  return (
    <div className="about-page">
      <Header title="Quake 3 EULA" className="about-header" />
      <div className="about-content">
        <div className="about-section">
          <h2>Quake 3 1.32 Patch Data</h2>
          <p>
            The 1.32 point-release patch data (<code>pak1.pk3</code>–
            <code>pak8.pk3</code> for <code>baseq3</code>,{" "}
            <code>pak1.pk3</code>–<code>pak3.pk3</code> for{" "}
            <code>missionpack</code>) is distributed by id Software under the
            license below. Read it through to the end, then check the box
            and download.
          </p>
          {loadError && (
            <p className="eula-load-error">
              Failed to load EULA text: {loadError}. Please refresh the page.
            </p>
          )}
          <div
            ref={boxRef}
            onScroll={handleScroll}
            className="eula-box"
            tabIndex={0}
          >
            {eula ?? "Loading…"}
          </div>
          <label
            className={`eula-agree ${scrolled ? "" : "eula-agree-locked"}`}
          >
            <input
              type="checkbox"
              checked={agreed}
              disabled={!scrolled}
              onChange={(e) => setAgreed(e.target.checked)}
            />
            <span>
              I have read and agree to the license above.
              {!scrolled && (
                <span className="eula-agree-hint">
                  {" "}
                  Scroll to the bottom of the license to enable.
                </span>
              )}
            </span>
          </label>
          <div className="eula-download">
            <a
              href={agreed ? PATCH_ZIP_URL : undefined}
              className={`eula-download-btn ${
                agreed ? "" : "eula-download-btn-disabled"
              }`}
              aria-disabled={!agreed}
              onClick={(e) => {
                if (!agreed) e.preventDefault();
              }}
              download
            >
              Download Quake 3 1.32 patches
            </a>
          </div>
        </div>
      </div>
    </div>
  );
}
