import { Link } from "react-router-dom";
import { MOVEMENT_MODES, GAMEPLAY_MODES } from "../ServerCard";

interface FeatureItem {
  name: string;
  cvar?: string;
  desc: string;
  video?: string;
}

const FEATURES: FeatureItem[] = [
  {
    name: "VR Tracking",
    desc: "1:1 head and weapon hand tracking in VR — your player's head and weapon move exactly as you do.",
    video: "vr_tracking",
  },
  {
    name: "Damage Plums",
    cvar: "cg_damagePlums 1",
    desc: "Floating damage numbers appear on each hit, showing exactly how much damage you dealt.",
    video: "cg_damagePlums",
  },
  {
    name: "Blood Particles",
    cvar: "cg_bloodParticles 1",
    desc: "Particle-based blood effects with wall and floor splats, replacing the default sprite blood.",
    video: "cg_bloodParticles",
  },
  {
    name: "Damage Effect",
    cvar: "cg_damageEffect 1",
    desc: "Directional red vignette when taking damage, replacing the default blood splatter overlay.",
    video: "cg_damageEffect",
  },
  {
    name: "Orbit Camera",
    cvar: "cg_followMode 1 / cg_smoothFollow 1",
    desc: "Third-person orbit camera for spectating.",
    video: "orbit_camera",
  },
  {
    name: "Stencil Shadows",
    cvar: "r_stencilbits 8 / r_shadows 2",
    desc: "Improved shadow volumes with BSP clipping to prevent wall and floor bleed-through. Tune with r_shadowDistance, r_shadowClip, r_shadowClipPenetration, and r_shadowClipExtension.",
  },
  {
    name: "TV Demo Scrubbing",
    cvar: "+tv_scrub",
    desc: "Scrub forward and backward through recorded TV demos.",
    video: "tvd_scrub",
  },
  {
    name: "TV Demo Pause",
    cvar: "demopause",
    desc: "Pause and resume TV demo playback.",
    video: "tvd_pause",
  },
  {
    name: "TV Demo Download",
    desc: "When a server has sv_tvDownload enabled, clients are offered demo downloads on map rotation.",
  },
];

export function DocsFeatures() {
  return (
    <>
      <div className="about-section">
        <h2>Trinity Features</h2>
        <div className="about-features">
          {FEATURES.map((f) => (
            <details key={f.name} className="about-feature">
              <summary>
                <span className="about-feature-name">{f.name}</span>
                {f.cvar && (
                  <code className="about-feature-cvar">{f.cvar}</code>
                )}
              </summary>
              <div className="about-feature-content">
                <p>{f.desc}</p>
                {f.video && (
                  <video autoPlay loop muted playsInline>
                    <source
                      src={`/assets/videos/${f.video}.webm`}
                      type="video/webm"
                    />
                    <source
                      src={`/assets/videos/${f.video}.mp4`}
                      type="video/mp4"
                    />
                  </video>
                )}
              </div>
            </details>
          ))}
        </div>
      </div>

      <div className="about-section">
        <h2>Gameplay Modes</h2>
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
        <h2>Movement Modes</h2>
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

      <div className="about-section">
        <h2>Authentication</h2>
        <p>
          Logging in links your game identity with your Trinity account,
          so your stats are automatically associated no matter which device
          or engine you play from.
        </p>
        <p>
          The easiest way to log in is from the main menu's{" "}
          <strong>Account</strong> option. You can
          also set it up manually by adding both <code>cl_trinityUser</code>{" "}
          and <code>cl_trinityToken</code> to your autoexec. Your game token
          can be found on your <Link to="/account">Account page</Link>.
        </p>
        <p>
          Once logged in, your credentials are included in the Trinity
          handshake when you connect to a server. Your GUID is automatically
          linked to your account, so you don't need to use{" "}
          <code>!claim</code> or <code>!link</code> for identities you play
          on while authenticated.
        </p>
      </div>
    </>
  );
}
