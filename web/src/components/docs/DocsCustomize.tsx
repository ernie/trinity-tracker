import { DocsH2 } from "./DocsH2";
import { MOVEMENT_MODES, GAMEPLAY_MODES } from "../ServerCard";

const CONFIG_DOWNLOADS = [
  {
    name: "Trinity Engine (Flatscreen)",
    href: "/configs/trinity-autoexec.cfg",
    desc: "Quake3e-based engine for desktop",
  },
  {
    name: "Quake 3 VR",
    href: "/configs/q3vr-autoexec.cfg",
    desc: "PCVR engine — includes controller bindings",
  },
  {
    name: "Quake3Quest",
    href: "/configs/ioq3quest-autoexec.cfg",
    desc: "Quest 2, 3, and 3S — includes controller bindings",
  },
];

// New /docs/customize page — Configuration (autoexec.cfg downloads)
// extracted from DocsGettingStarted, plus Gameplay/Movement Modes
// from DocsFeatures. Tone polish + 'how to tell what mode a server
// is running' content land in a later phase.
export function DocsCustomize() {
  return (
    <>
      <div className="about-section">
        <DocsH2 id="configuration">Configuration</DocsH2>
        <p>
          Trinity adds gameplay features beyond base Quake 3. Create an{" "}
          <code>autoexec.cfg</code> in your <code>baseq3</code> folder to
          enable them, or download a suggested starting point for your engine:
        </p>
        <div className="about-downloads">
          {CONFIG_DOWNLOADS.map((c) => (
            <a
              key={c.href}
              href={c.href}
              download="autoexec.cfg"
              className="about-download-item"
            >
              <div className="about-download-info">
                <span className="about-download-name">{c.name}</span>
                <span className="about-download-desc">{c.desc}</span>
              </div>
              <span className="about-download-version">autoexec.cfg</span>
            </a>
          ))}
        </div>
      </div>

      <div className="about-section">
        <DocsH2 id="gameplay-modes">Gameplay Modes</DocsH2>
        <p>
          Trinity servers can run alternative gameplay rules via
          the <code>g_gameplay</code> cvar. The active gameplay mode is shown
          on server and match cards.
        </p>
        <div className="docs-modes">
          {Object.entries(GAMEPLAY_MODES).map(([value, mode]) => (
            <div key={value} className="docs-mode-item">
              <img src={mode.icon} alt={mode.label} className="docs-mode-icon" />
              <div className="docs-mode-info">
                <span className="docs-mode-name">{mode.label}</span>
                <code className="docs-mode-value">g_gameplay {value}</code>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="about-section">
        <DocsH2 id="movement-modes">Movement Modes</DocsH2>
        <p>
          Movement physics can be changed independently of gameplay rules via
          the <code>g_movement</code> cvar. This controls how air control,
          strafe jumping, and other movement mechanics work.
        </p>
        <div className="docs-modes">
          {Object.entries(MOVEMENT_MODES).map(([value, mode]) => (
            <div key={value} className="docs-mode-item">
              <img src={mode.icon} alt={mode.label} className="docs-mode-icon" />
              <div className="docs-mode-info">
                <span className="docs-mode-name">{mode.label}</span>
                <code className="docs-mode-value">g_movement {value}</code>
              </div>
            </div>
          ))}
        </div>
      </div>
    </>
  );
}
