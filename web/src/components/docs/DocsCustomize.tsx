import { Link } from 'react-router-dom'
import { DocsH2 } from './DocsH2'
import { PlatformOnly } from './PlatformOnly'
import { PlatformNote } from './PlatformNote'
import { PlatformChip } from './PlatformChip'
import { DownloadIcon } from '../DownloadIcon'
import { MOVEMENT_MODES } from '../ServerCard'

// /docs/customize — Phase 3c rewrite. Replaces the Phase 2 lift
// (Configuration + raw mode tables) with cvar-by-cvar tunables
// organized by intent. Cvar inventory drawn from the curated starter
// autoexec configs in web/public/configs/, descriptions verified
// against trinity, q3vr, and ioq3quest cgame/engine sources.
// Descriptions are factual only — no editorial recommendations or
// "tune if it feels X" advice; cvars do what they do, the user
// decides.
export function DocsCustomize() {
  return (
    <>
      <div className="about-section">
        <DocsH2 id="where-settings-live">Where settings live</DocsH2>
        <p>
          Trinity reads two configs at startup, both in your install's{' '}
          <code>baseq3</code> folder:
        </p>
        <ul>
          <li>
            <strong><code>q3config.cfg</code></strong> — auto-saved
            on shutdown and overwritten by anything you change in
            the in-game menus. Don't edit this by hand; your
            changes get stomped next time the game closes.
          </li>
          <li>
            <strong><code>autoexec.cfg</code></strong> — read after{' '}
            <code>q3config.cfg</code> on every launch and never
            overwritten. This is where to put settings you want to
            persist: Trinity feature toggles, network tuning,
            bindings.
          </li>
        </ul>
        <PlatformNote platform="quest">
          <p>
            On Quest, both files live at{' '}
            <code>/sdcard/ioquake3Quest/baseq3/</code> on the
            headset's internal storage rather than inside the app
            install. The engine creates that directory on first
            launch.
          </p>
        </PlatformNote>
      </div>

      <div className="about-section">
        <DocsH2 id="starter-configs">Starter config</DocsH2>
        <p>
          A pre-curated <code>autoexec.cfg</code> for your
          platform — drop it into your <code>baseq3</code> folder
          and edit to taste.
        </p>
        <div className="about-downloads">
          <PlatformOnly platform="flatscreen">
            <a
              href="/configs/trinity-autoexec.cfg"
              download="autoexec.cfg"
              className="about-download-item"
            >
              <div className="about-download-info">
                <span className="about-download-name">Trinity Engine</span>
                <span className="about-download-desc">
                  Trinity feature toggles, online network values,
                  and Quake3e renderer presets.
                </span>
              </div>
              <DownloadIcon size={20} className="about-download-icon" />
            </a>
          </PlatformOnly>
          <PlatformOnly platform="pcvr">
            <a
              href="/configs/q3vr-autoexec.cfg"
              download="autoexec.cfg"
              className="about-download-item"
            >
              <div className="about-download-info">
                <span className="about-download-name">Quake 3 VR</span>
                <span className="about-download-desc">
                  Trinity feature toggles, online network values,
                  VR comfort settings, and Meta Touch controller
                  bindings.
                </span>
              </div>
              <DownloadIcon size={20} className="about-download-icon" />
            </a>
          </PlatformOnly>
          <PlatformOnly platform="quest">
            <a
              href="/configs/ioq3quest-autoexec.cfg"
              download="autoexec.cfg"
              className="about-download-item"
            >
              <div className="about-download-info">
                <span className="about-download-name">Quake3Quest</span>
                <span className="about-download-desc">
                  Trinity feature toggles, online network values,
                  VR comfort settings, Meta Touch controller
                  bindings, and Quest refresh-rate tuning.
                </span>
              </div>
              <DownloadIcon size={20} className="about-download-icon" />
            </a>
          </PlatformOnly>
        </div>
      </div>

      <div className="about-section">
        <DocsH2 id="trinity-features">Trinity feature toggles</DocsH2>
        <p>Trinity-introduced cvars.</p>
        <ul className="docs-cvars">
          <li>
            <code>cg_damagePlums 1</code> — floating damage numbers
            above each hit. Default <code>0</code>.
          </li>
          <li>
            <code>cg_hitSounds 1</code> — damage-scaled hit sound
            feedback. Default <code>0</code>.
            <ul>
              <li><code>0</code> off</li>
              <li><code>1</code> lower pitch on bigger hits</li>
              <li><code>2</code> higher pitch on bigger hits</li>
            </ul>
          </li>
          <li>
            <code>cg_damageEffect 1</code> — directional red
            vignette overlay when taking damage. Default{' '}
            <code>0</code>.
          </li>
          <li>
            <code>cg_bloodParticles 1</code> — particle blood with
            wall and floor splats. Default <code>0</code>.
          </li>
          <li>
            <code>cg_drawTimer 1</code> — match timer in the HUD.
            Default <code>0</code>.
          </li>
          <li>
            <code>cl_tvDownload 1</code> — TV demo download offer
            behavior at end of match. The prompt appears at the
            start of the next match. Default <code>1</code>.
            <ul>
              <li><code>0</code> off (no prompt)</li>
              <li>
                <code>1</code> prompt; defaults to decline if no
                response
              </li>
              <li>
                <code>2</code> prompt; defaults to accept if no
                response
              </li>
            </ul>
          </li>
          <li>
            <code>ui_trinitySigil 1</code> — 3D Trinity sigil in
            the main menus. <code>0</code> hides it. Default{' '}
            <code>1</code>.
          </li>
          <PlatformOnly platform="flatscreen">
            <li>
              <code>cg_followMode</code>{' '}
              <PlatformChip platform="flatscreen" /> — spectator
              camera mode.
              <ul>
                <li><code>0</code> first-person follow</li>
                <li><code>1</code> third-person orbit camera</li>
                <li><code>2</code> free-fly (TV playback only)</li>
              </ul>
            </li>
          </PlatformOnly>
          <PlatformOnly platform={['pcvr', 'quest']}>
            <li>
              <code>cg_smoothFollow 1</code>{' '}
              <PlatformChip platform={['pcvr', 'quest']} /> — orbit
              camera for VR third-person spectating. Default{' '}
              <code>0</code>: the third-person camera snaps to
              a new position when context changes (player switch,
              recenter). <code>1</code>: a continuous orbit you
              steer with the thumbstick — rotate around the
              player, zoom in/out, and press B to recenter.
              Lives in the in-game Comfort menu because
              continuous camera movement in VR takes strong VR
              legs.
            </li>
          </PlatformOnly>
        </ul>
      </div>

      <div className="about-section">
        <DocsH2 id="voice-chat">Voice chat</DocsH2>
        <p>
          Voice chat cvars. <code>cl_*</code> values are
          engine-side (recording, output volume, channel mutes);{' '}
          <code>cg_drawVoipSpeakers</code> is cgame-side (the
          on-HUD active-speaker list). See{' '}
          <Link to="/docs/install#troubleshooting">the Install
          page's troubleshooting section</Link> for what to do
          when voice isn't working.
        </p>
        <ul className="docs-cvars">
          <li>
            <code>cl_voip 1</code> — voice chat enabled
            client-side (default). <code>0</code> disables it
            entirely.
          </li>
          <li>
            <code>cl_voipVolume 1.0</code> — incoming voice
            playback volume, independent from{' '}
            <code>s_volume</code> and <code>s_musicvolume</code>.
          </li>
          <li>
            <code>cl_voipUseVAD 0</code> — voice transmission
            mode. Default <code>0</code>.
            <ul>
              <li>
                <code>0</code> push-to-talk — bind{' '}
                <code>+voiprecord</code> and hold to transmit
              </li>
              <li>
                <code>1</code> voice-activity detection:
                transmits when the mic picks up sound above the
                threshold
              </li>
            </ul>
          </li>
          <li>
            <code>cl_voipVADThreshold 0.1</code> — VAD activation
            threshold; only meaningful when{' '}
            <code>cl_voipUseVAD 1</code>.
          </li>
          <PlatformOnly platform="flatscreen">
            <li>
              <code>cl_voipShowMeter 1</code>{' '}
              <PlatformChip platform="flatscreen" /> — engine-drawn
              microphone-level meter on screen.
            </li>
          </PlatformOnly>
          <li>
            <code>cg_drawVoipSpeakers 1</code> — list of currently-
            speaking players in the upper-right of the HUD, with
            channel-colored speaker icons. <code>0</code> hides it.
          </li>
          <li>
            <code>cl_voipMuteAll</code> /{' '}
            <code>cl_voipMuteSpatial</code> /{' '}
            <code>cl_voipMuteDirect</code> /{' '}
            <code>cl_voipMuteTeam</code> — self-mute incoming
            voice on a per-channel basis (<code>1</code> mutes,{' '}
            <code>0</code> unmutes).
          </li>
        </ul>
        <p>
          Voice commands worth binding. <code>+voiprecord</code>{' '}
          and <code>voipvadtoggle</code> are an either/or —
          which you use depends on your{' '}
          <code>cl_voipUseVAD</code> mode:
        </p>
        <ul className="docs-cvars">
          <li>
            <code>+voiprecord</code> — hold to transmit while
            in push-to-talk mode (<code>cl_voipUseVAD 0</code>).
            Inert in VAD mode.
          </li>
          <li>
            <code>voipvadtoggle</code> — toggles{' '}
            <code>cl_voipVADMuted</code> (mic mute) on and off
            while in VAD mode (<code>cl_voipUseVAD 1</code>).
            Inert in push-to-talk mode.
          </li>
          <li>
            <code>voiptarget [spatial|team|all]</code> — sets{' '}
            <code>cl_voipSendTarget</code> directly, or cycles
            through the three with no argument.
          </li>
        </ul>
      </div>

      <div className="about-section">
        <DocsH2 id="modes-note">Movement &amp; gameplay modes</DocsH2>
        <p>
          Trinity ships with several Quake 3 movement and
          gameplay variants. Usually, they are matched together:
          Vanilla Q3 and Vanilla Q3, CPMA and CPMA, Quake Live
          (or Quake Live Turbo) and Quake Live, but you can mix
          and match if you like, and the choices you make in the
          menu will affect all of your local single player games.
        </p>
        <p>
          In multiplayer the server picks the rules — joining a
          server hands movement and gameplay over to whatever
          it's running. Pull up the scoreboard during a match to
          see what's active: the modes appear as small icons in
          the top-right of the HUD, and the server name usually
          calls them out too.
        </p>
        <p>The icons you'll see:</p>
        <div className="docs-modes">
          {Object.values(MOVEMENT_MODES).map((m) => (
            <div key={m.icon} className="docs-mode-item">
              <img src={m.icon} alt={m.label} className="docs-mode-icon" />
              <div className="docs-mode-info">
                <span className="docs-mode-name">{m.label}</span>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="about-section">
        <DocsH2 id="player-models">Forced enemy / team models</DocsH2>
        <p>
          Override what other players' models render as so that
          enemies and teammates are visually distinct. Skins
          suffixed with <code>/pm</code> (promode) or{' '}
          <code>/fb</code> (fullbright) accept the color
          overrides; stock skins ignore them.
        </p>
        <ul className="docs-cvars">
          <li>
            <code>cg_enemyModel "keel/pm"</code> — force every
            enemy to render as this model + skin pair. Empty
            disables.
          </li>
          <li>
            <code>cg_enemyColors "333"</code> — up to five
            characters: head, body, legs,{' '}
            <code>color1</code> (rail core / weapon glow),{' '}
            <code>color2</code> (rail spiral). A 3-char string
            sets only the model tints; positions 4 and 5 inherit
            the enemy's own rail colors if omitted.
            <ul>
              <li><code>1</code> blue</li>
              <li><code>2</code> green</li>
              <li><code>3</code> cyan</li>
              <li><code>4</code> red</li>
              <li><code>5</code> magenta</li>
              <li><code>6</code> yellow</li>
              <li><code>7</code> white</li>
              <li>
                <code>?</code> in any position becomes the
                player's team color — red, blue, or white (in
                FFA).
              </li>
            </ul>
          </li>
          <li>
            <code>cg_teamModel "doom/pm"</code> — same as{' '}
            <code>cg_enemyModel</code> but for teammates.
          </li>
          <li>
            <code>cg_teamColors "222"</code> — color triplet for
            teammate models.
          </li>
          <li>
            <code>cg_deadBodyDarken 1</code> — grey out dead
            bodies (requires pm/fb skins). Default <code>1</code>.
          </li>
        </ul>
      </div>

      <PlatformOnly platform={['pcvr', 'quest']}>
        <div className="about-section">
          <DocsH2 id="vr" platforms={['pcvr', 'quest']}>
            VR comfort &amp; controllers
          </DocsH2>
          <p>
            VR-specific cvars. Controller bindings live in your
            starter config under the <code>vr_button_map_*</code>{' '}
            entries.
          </p>
          <ul className="docs-cvars">
            <li>
              <code>vr_snapturn 0</code> — snap turn angle in
              degrees.
              <ul>
                <li><code>0</code> smooth turn</li>
                <li>
                  <code>N</code> (any nonzero value, e.g.{' '}
                  <code>45</code>) snap by N degrees per stick
                  flick
                </li>
              </ul>
            </li>
            <li>
              <code>vr_directionMode 0</code> /{' '}
              <code>1</code> — strafe direction reference. The
              two modes follow either head orientation or off-hand
              controller orientation; the in-game Controls menu
              labels them explicitly.
            </li>
            <li>
              <code>vr_twoHandedWeapons N</code> — two-hand
              weapon stabilization mode (off, single-handed, or
              two-handed). Hold the secondary grip to stabilize.
            </li>
            <li>
              <code>vr_thumbstickDeadzone 0.1</code> /{' '}
              <code>vr_thumbstickFullDeflection 0.85</code> —
              thumbstick response curve endpoints.
            </li>
            <li>
              <code>vr_triggerSensitivity 0.9</code> — trigger
              pull threshold for fire (<code>0.0</code>–
              <code>1.0</code>).
            </li>
            <li>
              <code>vr_hudScale 1.5</code> — HUD size multiplier
              in VR.
            </li>
          </ul>
          <PlatformOnly platform="quest">
            <p>
              Quest-only:{' '}
              <code>vr_refreshrate 120</code> sets the headset
              display refresh rate; <code>com_maxfps 0</code>{' '}
              uncaps the engine's frame rate so it tracks the
              headset.
            </p>
          </PlatformOnly>
          <PlatformOnly platform="pcvr">
            <p>
              PCVR-only: <code>vr_hudDepth 3</code> sets how far
              out the HUD renders in the field of view.
            </p>
          </PlatformOnly>
        </div>
      </PlatformOnly>

      <div className="about-section">
        <DocsH2 id="network">Network tuning</DocsH2>
        <ul className="docs-cvars">
          <li>
            <code>cl_maxpackets 125</code> — outgoing packet rate
            ceiling, in packets per second.
          </li>
          <li>
            <code>snaps 40</code> — requested snapshot rate from
            the server.
          </li>
          <li>
            <code>rate 50000</code> — bandwidth budget for the
            connection, in bytes per second.
          </li>
          <li>
            <code>cl_guidServerUniq 0</code> — keep your{' '}
            <code>qkey</code>-derived identity stable across
            servers. <code>1</code> generates a different identity
            per server. See{' '}
            <Link to="/docs/account">the Account page</Link> for
            background on identity.
          </li>
        </ul>
      </div>
    </>
  )
}
