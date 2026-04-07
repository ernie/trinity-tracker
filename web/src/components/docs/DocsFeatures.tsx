import { Link } from "react-router-dom";
import { MOVEMENT_MODES, GAMEPLAY_MODES } from "../ServerCard";

export function DocsFeatures() {
  return (
    <>
      <div className="about-section">
        <h2>Trinity Features</h2>
        <div className="about-features">
          <details className="about-feature">
            <summary>
              <span className="about-feature-name">VR Tracking</span>
            </summary>
            <div className="about-feature-content">
              <p>
                1:1 head and weapon hand tracking in VR — your player's head
                and weapon move exactly as you do.
              </p>
              <video autoPlay loop muted playsInline>
                <source src="/assets/videos/vr_tracking.webm" type="video/webm" />
                <source src="/assets/videos/vr_tracking.mp4" type="video/mp4" />
              </video>
            </div>
          </details>

          <details className="about-feature">
            <summary>
              <span className="about-feature-name">Damage Plums</span>
              <code className="about-feature-cvar">cg_damagePlums 1</code>
            </summary>
            <div className="about-feature-content">
              <p>
                Floating damage numbers appear on each hit, showing exactly
                how much damage you dealt.
              </p>
              <video autoPlay loop muted playsInline>
                <source src="/assets/videos/cg_damagePlums.webm" type="video/webm" />
                <source src="/assets/videos/cg_damagePlums.mp4" type="video/mp4" />
              </video>
            </div>
          </details>

          <details className="about-feature">
            <summary>
              <span className="about-feature-name">Blood Particles</span>
              <code className="about-feature-cvar">cg_bloodParticles 1</code>
            </summary>
            <div className="about-feature-content">
              <p>
                Particle-based blood effects with wall and floor splats,
                replacing the default sprite blood.
              </p>
              <video autoPlay loop muted playsInline>
                <source src="/assets/videos/cg_bloodParticles.webm" type="video/webm" />
                <source src="/assets/videos/cg_bloodParticles.mp4" type="video/mp4" />
              </video>
            </div>
          </details>

          <details className="about-feature">
            <summary>
              <span className="about-feature-name">Damage Effect</span>
              <code className="about-feature-cvar">cg_damageEffect 1</code>
            </summary>
            <div className="about-feature-content">
              <p>
                Directional red vignette when taking damage, replacing the
                default blood splatter overlay.
              </p>
              <video autoPlay loop muted playsInline>
                <source src="/assets/videos/cg_damageEffect.webm" type="video/webm" />
                <source src="/assets/videos/cg_damageEffect.mp4" type="video/mp4" />
              </video>
            </div>
          </details>

          <details className="about-feature">
            <summary>
              <span className="about-feature-name">Orbit Camera</span>
              <code className="about-feature-cvar">
                cg_followMode 1 / cg_smoothFollow 1
              </code>
            </summary>
            <div className="about-feature-content">
              <p>Third-person orbit camera for spectating.</p>
              <video autoPlay loop muted playsInline>
                <source src="/assets/videos/orbit_camera.webm" type="video/webm" />
                <source src="/assets/videos/orbit_camera.mp4" type="video/mp4" />
              </video>
            </div>
          </details>

          <details className="about-feature">
            <summary>
              <span className="about-feature-name">TV Demo Scrubbing</span>
              <code className="about-feature-cvar">+tv_scrub</code>
            </summary>
            <div className="about-feature-content">
              <p>Scrub forward and backward through recorded TV demos.</p>
              <video autoPlay loop muted playsInline>
                <source src="/assets/videos/tvd_scrub.webm" type="video/webm" />
                <source src="/assets/videos/tvd_scrub.mp4" type="video/mp4" />
              </video>
            </div>
          </details>

          <details className="about-feature">
            <summary>
              <span className="about-feature-name">TV Demo Pause</span>
              <code className="about-feature-cvar">demopause</code>
            </summary>
            <div className="about-feature-content">
              <p>Pause and resume TV demo playback.</p>
              <video autoPlay loop muted playsInline>
                <source src="/assets/videos/tvd_pause.webm" type="video/webm" />
                <source src="/assets/videos/tvd_pause.mp4" type="video/mp4" />
              </video>
            </div>
          </details>

          <div className="about-feature about-feature-flat">
            <div className="about-feature-header">
              <span className="about-feature-name">Stencil Shadows</span>
              <code className="about-feature-cvar">r_shadows 2</code>
            </div>
            <div className="about-feature-content">
              <p>
                Shadow volumes with BSP clipping to prevent wall and floor
                bleed-through. Tune with <code>r_shadowDistance</code>,{" "}
                <code>r_shadowClip</code>,{" "}
                <code>r_shadowClipPenetration</code>, and{" "}
                <code>r_shadowClipExtension</code>.
              </p>
            </div>
          </div>

          <div className="about-feature about-feature-flat">
            <div className="about-feature-header">
              <span className="about-feature-name">Voice Chat</span>
              <code className="about-feature-cvar">cl_voip 1</code>
            </div>
            <div className="about-feature-content">
              <p>
                Opus-based voice chat on supported servers, also played back
                in TV demos. You can speak either push-to-talk or on voice
                activity, and control who hears you and what you hear per
                channel.
              </p>
              <p>
                <strong>Speaking:</strong>
              </p>
              <ul>
                <li>
                  <code>bind &lt;key&gt; +voiprecord</code> — push-to-talk
                  while held.
                </li>
                <li>
                  <code>cl_voipUseVAD 1</code> — transmit automatically on
                  voice activity. Tune the trigger level with{" "}
                  <code>cl_voipVADThreshold</code>.
                </li>
                <li>
                  <code>bind &lt;key&gt; voipvadtoggle</code> — quick
                  mute/unmute of your mic while in VAD mode.
                </li>
                <li>
                  <code>voiptarget</code> — cycle who hears you (spatial →
                  team → all). Use <code>voiptarget spatial|team|all</code>{" "}
                  to set it explicitly.
                </li>
              </ul>
              <p>
                <strong>Channel colors:</strong> the speaker indicator and
                send-channel display use a consistent color key:
              </p>
              <ul className="voip-channel-key">
                <li>
                  <span
                    className="voip-channel-swatch"
                    style={{ background: "rgb(255, 255, 51)" }}
                  />
                  <span>
                    Spatial — nearby players hear you positionally
                  </span>
                </li>
                <li>
                  <span
                    className="voip-channel-swatch"
                    style={{ background: "rgb(51, 255, 255)" }}
                  />
                  <span>Team</span>
                </li>
                <li>
                  <span
                    className="voip-channel-swatch"
                    style={{ background: "rgb(51, 255, 51)" }}
                  />
                  <span>All</span>
                </li>
                <li>
                  <span
                    className="voip-channel-swatch"
                    style={{ background: "rgb(255, 51, 255)" }}
                  />
                  <span>
                    Direct — private to a specific client. Not part of the{" "}
                    <code>voiptarget</code> cycle; reached by setting{" "}
                    <code>cl_voipSendTarget</code> to a specific client ID,{" "}
                    <code>crosshair</code>, or <code>attacker</code>.
                  </span>
                </li>
              </ul>
              <p>
                <strong>Listening:</strong>
              </p>
              <ul>
                <li>
                  <code>cl_voipVolume</code> — incoming volume (0.0–2.0,
                  allows boost).
                </li>
                <li>
                  <code>cl_voipMuteSpatial</code>,{" "}
                  <code>cl_voipMuteTeam</code>,{" "}
                  <code>cl_voipMuteDirect</code>,{" "}
                  <code>cl_voipMuteAll</code> — mute channels independently.
                </li>
                <li>
                  <code>voip ignore &lt;id&gt;</code> /{" "}
                  <code>voip unignore &lt;id&gt;</code> — silence a specific
                  player.
                </li>
                <li>
                  <code>voip gain &lt;id&gt; &lt;value&gt;</code> — adjust
                  per-player volume.
                </li>
              </ul>
              <p>
                <strong>VR (grip weapon wheel scheme):</strong>
              </p>
              <p>
                The primary thumbstick press and its alt layer are unbound
                in this scheme, so they're the natural place for voice
                controls. From the console:
              </p>
              <ul>
                <li>
                  <code>
                    vr_button_map_PRIMARYTHUMBSTICK "+voiprecord"
                  </code>{" "}
                  for push-to-talk, or{" "}
                  <code>
                    vr_button_map_PRIMARYTHUMBSTICK "voipvadtoggle"
                  </code>{" "}
                  if you prefer VAD with a quick mic mute.
                </li>
                <li>
                  <code>
                    vr_button_map_PRIMARYTHUMBSTICK_ALT "voiptarget"
                  </code>{" "}
                  — rest your thumb on the thumbrest (which triggers the
                  alt layer by default) and click the primary thumbstick to
                  cycle channel targets.
                </li>
              </ul>
              <p>
                Set <code>cl_voip 0</code> to disable voice chat entirely.
              </p>
            </div>
          </div>

          <div className="about-feature about-feature-flat">
            <div className="about-feature-header">
              <span className="about-feature-name">TV Demo Download</span>
            </div>
            <div className="about-feature-content">
              <p>
                When a server has <code>sv_tvDownload</code> enabled, clients
                are offered demo downloads on map rotation.
              </p>
            </div>
          </div>
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
