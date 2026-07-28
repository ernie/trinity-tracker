import { Link } from "react-router-dom";
import { DocsH2 } from "./DocsH2";
import { PlatformOnly } from "./PlatformOnly";
import { PlatformNote } from "./PlatformNote";
import { PlatformChip } from "./PlatformChip";
import { DownloadIcon } from "../DownloadIcon";
import { MODE_PROFILES } from "../ServerCard";

// /docs/customize — Phase 3c rewrite. Replaces the Phase 2 lift
// (Configuration + raw mode tables) with cvar-by-cvar tunables
// organized by intent. Cvar inventory drawn from the curated starter
// autoexec configs in web/public/configs/, descriptions verified
// against trinity, trinity-vr, and trinity-quest cgame/engine sources.
// Descriptions are factual only — no editorial recommendations or
// "tune if it feels X" advice; cvars do what they do, the user
// decides.
export function DocsCustomize() {
  return (
    <>
      <div className="about-section">
        <DocsH2 id="where-settings-live">Where settings live</DocsH2>
        <p>
          Trinity reads two configs at startup, both in your install's{" "}
          <code>baseq3</code> folder:
        </p>
        <ul>
          <li>
            <strong>
              <code>q3config.cfg</code>
            </strong>{" "}
            — auto-saved on shutdown and overwritten by anything you change in
            the in-game menus. Don't edit this by hand; your changes get stomped
            next time the game closes.
          </li>
          <li>
            <strong>
              <code>autoexec.cfg</code>
            </strong>{" "}
            — read after <code>q3config.cfg</code> on every launch and never
            overwritten. This is where to put settings you want to persist:
            Trinity feature toggles, network tuning, bindings.
          </li>
        </ul>
        <PlatformNote platform="quest">
          <p>
            On Quest, both files live at{" "}
            <code>/sdcard/ioquake3Quest/baseq3/</code> on the headset's internal
            storage rather than inside the app install. The engine creates that
            directory on first launch.
          </p>
        </PlatformNote>
      </div>

      <div className="about-section">
        <DocsH2 id="starter-configs">Starter config</DocsH2>
        <p>
          A pre-curated <code>autoexec.cfg</code> for your platform — drop it
          into your <code>baseq3</code> folder and edit to taste.
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
                  Trinity feature toggles, online network values, and Quake3e
                  renderer presets.
                </span>
              </div>
              <DownloadIcon size={20} className="about-download-icon" />
            </a>
          </PlatformOnly>
          <PlatformOnly platform="pcvr">
            <a
              href="/configs/trinity-vr-autoexec.cfg"
              download="autoexec.cfg"
              className="about-download-item"
            >
              <div className="about-download-info">
                <span className="about-download-name">Trinity VR</span>
                <span className="about-download-desc">
                  Trinity feature toggles, online network values, VR comfort
                  settings, and Meta Touch controller bindings.
                </span>
              </div>
              <DownloadIcon size={20} className="about-download-icon" />
            </a>
          </PlatformOnly>
          <PlatformOnly platform="quest">
            <a
              href="/configs/trinity-quest-autoexec.cfg"
              download="autoexec.cfg"
              className="about-download-item"
            >
              <div className="about-download-info">
                <span className="about-download-name">Trinity Quest</span>
                <span className="about-download-desc">
                  Trinity feature toggles, online network values, VR comfort
                  settings, Meta Touch controller bindings, and Quest
                  refresh-rate tuning.
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
            <code>cg_damagePlums 1</code> — floating damage numbers above each
            hit. Default <code>0</code>.
          </li>
          <li>
            <code>cg_damagePlumScale 1.5</code> — size multiplier for the damage
            numbers, for screens where they read small. Default <code>1.0</code>
            .
          </li>
          <li>
            <code>cg_hitSounds 1</code> — damage-scaled hit sound feedback.
            Default <code>0</code>.
            <ul>
              <li>
                <code>0</code> off
              </li>
              <li>
                <code>1</code> lower pitch on bigger hits
              </li>
              <li>
                <code>2</code> higher pitch on bigger hits
              </li>
            </ul>
          </li>
          <li>
            <code>cg_damageEffect 1</code> — directional red vignette overlay
            when taking damage. Default <code>0</code>.
          </li>
          <li>
            <code>com_blood 2</code> — modern blood that reacts to how hard you
            hit, with spray, lingering trails, and marks on nearby surfaces.{" "}
            <code>1</code> is classic sprite blood, <code>0</code> off. Default{" "}
            <code>2</code>.
          </li>
          <li>
            <code>cg_drawTimer 1</code> — match timer in the HUD. Default{" "}
            <code>0</code>.
          </li>
          <li>
            <code>cl_tvDownload 1</code> — TV demo download offer behavior at
            end of match. The prompt appears at the start of the next match.
            Default <code>1</code>.
            <ul>
              <li>
                <code>0</code> off (no prompt)
              </li>
              <li>
                <code>1</code> prompt; defaults to decline if no response
              </li>
              <li>
                <code>2</code> prompt; defaults to accept if no response
              </li>
            </ul>
          </li>
          <li>
            <code>ui_trinitySigil 1</code> — 3D Trinity sigil in the main menus.{" "}
            <code>0</code> hides it. Default <code>1</code>.
          </li>
          <PlatformOnly platform="flatscreen">
            <li>
              <code>cg_followMode</code> <PlatformChip platform="flatscreen" />{" "}
              — which camera the follow view starts in. Toggling it in-game
              writes back to the cvar, so this is the default you come back to.
              Also under <strong>Setup → Game Options → Follow Camera</strong>.
              <ul>
                <li>
                  <code>0</code> first-person follow
                </li>
                <li>
                  <code>1</code> third-person orbit camera
                </li>
                <li>
                  <code>2</code> free-fly (TV playback only)
                </li>
              </ul>
            </li>
          </PlatformOnly>
          <PlatformOnly platform={["pcvr", "quest"]}>
            <li>
              <code>cg_smoothFollow 1</code>{" "}
              <PlatformChip platform={["pcvr", "quest"]} /> — orbit camera for
              VR third-person spectating. Default <code>0</code>: the
              third-person camera snaps to a new position when context changes
              (player switch, recenter). <code>1</code>: a continuous orbit you
              steer with the thumbstick — rotate around the player, zoom in/out,
              and press B to recenter. Lives under{" "}
              <strong>VR Options → Comfort</strong> because continuous camera
              movement in VR takes strong VR legs.
            </li>
          </PlatformOnly>
        </ul>
      </div>

      <div className="about-section">
        <DocsH2 id="display-output">Display output</DocsH2>
        <PlatformOnly platform="flatscreen">
          <h3>Renderer</h3>
          <p>
            Trinity ships three renderers and picks one at launch with{" "}
            <code>cl_renderer</code>: <code>vulkan</code> (the default),{" "}
            <code>opengl2</code>, and <code>opengl</code> (the GL 1.1-compatible
            legacy path, for very old hardware). It's latched — set it in{" "}
            <code>autoexec.cfg</code>, or run <code>vid_restart</code> after
            changing it. HDR output below is Vulkan-only.
          </p>
        </PlatformOnly>
        <PlatformOnly platform="pcvr">
          <h3>Renderer</h3>
          <p>
            Trinity VR ships as one binary with two renderers, picked at launch
            with <code>cl_renderer</code>: <code>vulkan</code> (the default) and{" "}
            <code>opengl2</code>. It's latched — set it in{" "}
            <code>autoexec.cfg</code>, or run <code>vid_restart</code> after
            changing it. HDR output below is Vulkan-only.
          </p>
        </PlatformOnly>
        <PlatformOnly platform={["flatscreen", "pcvr"]}>
          <h3>HDR</h3>
          <p>
            On an HDR display, Trinity can output true HDR — brighter, more
            lifelike highlights on lights, explosions, plasma, and sky — instead
            of standard dynamic range. It needs the Vulkan renderer and HDR
            turned on in your OS display settings, and it ships off by default.
          </p>
        </PlatformOnly>
        <PlatformOnly platform="flatscreen">
          <p>
            Turn it on under <strong>Setup → Graphics → HDR Display</strong>,
            then run <code>vid_restart</code>. Calibrate under{" "}
            <strong>Setup → Display → HDR Calibration</strong>: raise{" "}
            <strong>Peak</strong> until the inner rectangle's edge just vanishes
            into the outer one — that is the brightness your display actually
            reaches. Set <strong>Highlight</strong> to taste.{" "}
            <code>r_hdrActive</code> reads <code>1</code> once HDR is genuinely
            live.
          </p>
          <p>
            Prefer the config file? Set <code>r_hdrDisplay 1</code> and{" "}
            <code>r_hdrPeak</code> (your calibrated peak in nits) in{" "}
            <code>autoexec.cfg</code>. Paper-white is automatic by default;{" "}
            <code>r_hdrSaturation</code>, <code>r_hdrSaturationFull</code>, and{" "}
            <code>r_hdrSoftKnee</code> are advanced knobs covered in the{" "}
            <Link to="/docs/reference">reference</Link>.
          </p>
        </PlatformOnly>
        <PlatformOnly platform="pcvr">
          <p>
            HDR applies to the <strong>desktop mirror window</strong> only, not
            the headset — the headset uses Rec.709 color.
          </p>
          <p>
            Turn it on under <strong>Setup → Graphics → HDR Display</strong>,
            then run <code>vid_restart</code>. Both menu rows gray out unless{" "}
            <code>cl_renderer</code> is <code>vulkan</code>.
          </p>
          <p>
            For <code>r_hdrPeak</code>, even easier: grab the number from a
            flatscreen install. It's the same display, so it's the same number.
            The screen is here too —{" "}
            <strong>Setup → Display → HDR Calibration</strong>, raise{" "}
            <strong>Peak</strong> until the inner rectangle's edge just vanishes
            into the outer one. The mirror isn't HDR as the headset renders it,
            so slide the headset up and read the pattern off the monitor.
          </p>
          <p>
            Prefer the config file? Set <code>r_hdrDisplay 1</code> and{" "}
            <code>r_hdrPeak</code> (your calibrated peak in nits) in{" "}
            <code>autoexec.cfg</code>.
          </p>
        </PlatformOnly>
        <PlatformOnly platform="quest">
          <p>
            Quest does not do HDR. Its headset uses Rec.709 color management,
            handled automatically, which keeps the wide-gamut panel from
            over-saturating the game's colors. There is nothing to set.
          </p>
        </PlatformOnly>
      </div>

      <div className="about-section">
        <DocsH2 id="voice-chat">Voice chat</DocsH2>
        <p>
          Voice chat cvars. <code>cl_*</code> values are engine-side (recording,
          output volume, channel mutes); <code>cg_drawVoipSpeakers</code> is
          cgame-side (the on-HUD active-speaker list). See{" "}
          <Link to="/docs/install#troubleshooting">
            the Install page's troubleshooting section
          </Link>{" "}
          for what to do when voice isn't working.
        </p>
        <ul className="docs-cvars">
          <li>
            <code>cl_voip 1</code> — voice chat enabled client-side (default).{" "}
            <code>0</code> disables it entirely.
          </li>
          <li>
            <code>cl_voipVolume 1.0</code> — incoming voice playback volume,
            independent from <code>s_volume</code> and{" "}
            <code>s_musicvolume</code>.
          </li>
          <li>
            <code>cl_voipUseVAD 0</code> — voice transmission mode. Default{" "}
            <code>0</code>.
            <ul>
              <li>
                <code>0</code> push-to-talk — bind <code>+voiprecord</code> and
                hold to transmit
              </li>
              <li>
                <code>1</code> voice-activity detection: transmits when the mic
                picks up sound above the threshold
              </li>
            </ul>
          </li>
          <li>
            <code>cl_voipVADThreshold 0.1</code> — VAD activation threshold;
            only meaningful when <code>cl_voipUseVAD 1</code>.
          </li>
          <PlatformOnly platform="flatscreen">
            <li>
              <code>cl_voipShowMeter 1</code>{" "}
              <PlatformChip platform="flatscreen" /> — engine-drawn
              microphone-level meter on screen.
            </li>
          </PlatformOnly>
          <li>
            <code>cg_drawVoipSpeakers 1</code> — list of currently- speaking
            players in the upper-right of the HUD, with channel-colored speaker
            icons. <code>0</code> hides it.
          </li>
          <li>
            <code>cl_voipMuteAll</code> / <code>cl_voipMuteSpatial</code> /{" "}
            <code>cl_voipMuteDirect</code> / <code>cl_voipMuteTeam</code> —
            self-mute incoming voice on a per-channel basis (<code>1</code>{" "}
            mutes, <code>0</code> unmutes).
          </li>
        </ul>
        <p>
          Voice commands worth binding. <code>+voiprecord</code> and{" "}
          <code>voipvadtoggle</code> are an either/or — which you use depends on
          your <code>cl_voipUseVAD</code> mode:
        </p>
        <ul className="docs-cvars">
          <li>
            <code>+voiprecord</code> — hold to transmit while in push-to-talk
            mode (<code>cl_voipUseVAD 0</code>). Inert in VAD mode.
          </li>
          <li>
            <code>voipvadtoggle</code> — toggles <code>cl_voipVADMuted</code>{" "}
            (mic mute) on and off while in VAD mode (
            <code>cl_voipUseVAD 1</code>). Inert in push-to-talk mode.
          </li>
          <li>
            <code>voiptarget [spatial|team|all]</code> — sets{" "}
            <code>cl_voipSendTarget</code> directly, or cycles through the three
            with no argument.
          </li>
        </ul>
      </div>

      <div className="about-section">
        <DocsH2 id="modes-note">Game modes</DocsH2>
        <p>
          Trinity bundles four game modes — Vanilla Q3, CPMA, Quake Live, and
          Quake Live Turbo. Each sets movement physics and combat rules,
          selected with the latched <code>g_mode</code> cvar (0–3, applied on{" "}
          <code>map_restart</code>). There's no in-game menu for it; set it from
          the console for your local single-player games.
        </p>
        <p>
          In multiplayer the server picks the mode. The server browser shows
          each server's mode as an icon to the left of its name; in-match, pull
          up the scoreboard to see it as a small icon in the top-right of the
          HUD.
        </p>
        <p>The icons you'll see:</p>
        <div className="docs-modes">
          {Object.values(MODE_PROFILES).map((m) => (
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
          Override what other players' models render as so that enemies and
          teammates are visually distinct. Skins suffixed with <code>/pm</code>{" "}
          (promode) or <code>/fb</code> (fullbright) accept the color overrides;
          stock skins ignore them.
        </p>
        <ul className="docs-cvars">
          <li>
            <code>cg_enemyModel "keel/pm"</code> — force every enemy to render
            as this model + skin pair. Empty disables.
          </li>
          <li>
            <code>cg_enemyColors "333"</code> — up to five characters: head,
            body, legs, <code>color1</code> (rail core / weapon glow),{" "}
            <code>color2</code> (rail spiral). A 3-char string sets only the
            model tints; positions 4 and 5 inherit the enemy's own rail colors
            if omitted. <a href="#color-codes">Color codes</a> below has the
            digit-to-color table.
          </li>
          <li>
            <code>cg_teamModel "doom/pm"</code> — same as{" "}
            <code>cg_enemyModel</code> but for teammates.
          </li>
          <li>
            <code>cg_teamColors "222"</code> — color triplet for teammate
            models.
          </li>
          <li>
            <code>cg_deadBodyDarken 1</code> — grey out dead bodies (requires
            pm/fb skins). Default <code>1</code>.
          </li>
        </ul>
      </div>

      <div className="about-section">
        <DocsH2 id="color-codes">Color codes</DocsH2>
        <p>
          Three things in Quake 3 use single-digit color codes, and they work
          independently — the color slider in the player-settings menu, the{" "}
          <code>color1</code> / <code>color2</code> cvars (and the force-model
          overrides built on them), and the <code>^N</code> chat / player-name
          escapes. The codes look the same on the surface but disagree about
          which digit means which color, so it's easy to write <code>"4"</code>{" "}
          in one place expecting red and get something else somewhere else. This
          section is the translation table.
        </p>

        <h3>The player-settings color slider</h3>
        <p>
          The in-game settings menu shows seven color swatches in rainbow
          spectrum order. Pick the color you want; the menu writes the
          corresponding digit to <code>color1</code> for you.
        </p>
        <div className="docs-table-scroll">
          <table className="docs-color-table">
            <thead>
              <tr>
                <th>Slot</th>
                <th>Visible swatch</th>
                <th>
                  Stored <code>color1</code>
                </th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>0</td>
                <td>red</td>
                <td>4</td>
              </tr>
              <tr>
                <td>1</td>
                <td>yellow</td>
                <td>6</td>
              </tr>
              <tr>
                <td>2</td>
                <td>green</td>
                <td>2</td>
              </tr>
              <tr>
                <td>3</td>
                <td>cyan</td>
                <td>3</td>
              </tr>
              <tr>
                <td>4</td>
                <td>blue</td>
                <td>1</td>
              </tr>
              <tr>
                <td>5</td>
                <td>magenta</td>
                <td>5</td>
              </tr>
              <tr>
                <td>6</td>
                <td>white</td>
                <td>7</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p>
          You don't have to memorize the digits — the slider is there precisely
          so you don't. The numbers matter when you're hand-editing a config or
          setting <code>cg_enemyColors</code>, both of which skip the slider.
        </p>

        <h3>
          <code>color1</code> / <code>color2</code> +{" "}
          <code>cg_enemyColors</code> / <code>cg_teamColors</code>
        </h3>
        <p>
          When you type these cvars by hand, the digit-to-color mapping is the
          table below. <code>color1</code> drives your rail core, weapon glow,
          and muzzle flash; <code>color2</code> drives the rail spiral.{" "}
          <code>cg_enemyColors</code> and <code>cg_teamColors</code> use the
          same digits in a five-character string (head, body, legs, color1,
          color2); omitted positions inherit the player's own settings.
        </p>
        <div className="docs-table-scroll">
          <table className="docs-color-table">
            <thead>
              <tr>
                <th>Digit</th>
                <th>Color</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <code>1</code>
                </td>
                <td>blue</td>
              </tr>
              <tr>
                <td>
                  <code>2</code>
                </td>
                <td>green</td>
              </tr>
              <tr>
                <td>
                  <code>3</code>
                </td>
                <td>cyan</td>
              </tr>
              <tr>
                <td>
                  <code>4</code>
                </td>
                <td>red</td>
              </tr>
              <tr>
                <td>
                  <code>5</code>
                </td>
                <td>magenta</td>
              </tr>
              <tr>
                <td>
                  <code>6</code>
                </td>
                <td>yellow</td>
              </tr>
              <tr>
                <td>
                  <code>7</code>
                </td>
                <td>white</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p>
          A <code>?</code> placeholder anywhere in the string becomes the digit
          for the player's team color at runtime — <code>'4'</code> on red,{" "}
          <code>'1'</code> on blue, <code>'7'</code> in FFA — so one config
          string stays team-aware across matches.
        </p>

        <h3>
          Chat / name <code>^N</code> color escapes
        </h3>
        <p>
          For text — chat, player names, frag feed, scoreboard, menus — escapes
          like <code>^1text</code> use a different mapping entirely. This is the
          one most players have memorized from years of typing colored names and
          chat:
        </p>
        <div className="docs-table-scroll">
          <table className="docs-color-table">
            <thead>
              <tr>
                <th>Escape</th>
                <th>Color</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <code>^0</code>
                </td>
                <td>black</td>
              </tr>
              <tr>
                <td>
                  <code>^1</code>
                </td>
                <td>red</td>
              </tr>
              <tr>
                <td>
                  <code>^2</code>
                </td>
                <td>green</td>
              </tr>
              <tr>
                <td>
                  <code>^3</code>
                </td>
                <td>yellow</td>
              </tr>
              <tr>
                <td>
                  <code>^4</code>
                </td>
                <td>blue</td>
              </tr>
              <tr>
                <td>
                  <code>^5</code>
                </td>
                <td>cyan</td>
              </tr>
              <tr>
                <td>
                  <code>^6</code>
                </td>
                <td>magenta</td>
              </tr>
              <tr>
                <td>
                  <code>^7</code>
                </td>
                <td>white</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p>
          Only <code>2</code> (green) and <code>7</code> (white) mean the same
          color in both this table and the cvar table above. Everything else
          differs — most painfully, <code>1</code> and <code>4</code> swap. Chat{" "}
          <code>^1</code> is red, but <code>color1 1</code> is blue. Chat{" "}
          <code>^4</code> is blue, but <code>color1 4</code> is red. If you put{" "}
          <code>^1MyName</code> in your player name expecting your rail to
          match, it won't.
        </p>
        <p>
          For names and chat that look the same to players on every engine, stay
          within <code>^0</code>–<code>^7</code>.
        </p>
      </div>

      <PlatformOnly platform={["pcvr", "quest"]}>
        <div className="about-section">
          <DocsH2 id="vr" platforms={["pcvr", "quest"]}>
            VR comfort &amp; controllers
          </DocsH2>
          <p>
            VR-specific cvars. Most have a menu row under{" "}
            <strong>Setup → VR Options</strong> — <strong>Controls</strong> for
            aiming and input, <strong>Comfort</strong> for vignette and turning,{" "}
            <strong>HUD &amp; Display</strong> for HUD placement. The thumbstick
            response endpoints are autoexec-only. Controller bindings live in
            your starter config under the <code>vr_button_map_*</code> entries.
          </p>
          <ul className="docs-cvars">
            <li>
              <code>vr_snapturn 0</code> — snap turn angle in degrees.
              <ul>
                <li>
                  <code>0</code> smooth turn
                </li>
                <li>
                  <code>N</code> (any nonzero value, e.g. <code>45</code>) snap
                  by N degrees per stick flick
                </li>
              </ul>
            </li>
            <li>
              <code>vr_directionMode 0</code> / <code>1</code> — movement
              direction reference. The two modes follow either head orientation
              or off-hand controller orientation;{" "}
              <strong>VR Options → Controls → Direction Mode</strong> labels
              them explicitly.
            </li>
            <li>
              <code>vr_twoHandedWeapons N</code> — two-handed weapon grip; hold
              the secondary grip to stabilize the weapon. Default <code>0</code>
              .
              <ul>
                <li>
                  <code>0</code> off — one-handed aiming
                </li>
                <li>
                  <code>1</code> basic — the weapon aims along the line from
                  your primary controller toward your secondary controller
                </li>
                <li>
                  <code>2</code> virtual gun stock — the weapon anchors near
                  your face, like sighting along a stock
                </li>
              </ul>
            </li>
            <li>
              <code>vr_thumbstickDeadzone 0.1</code> /{" "}
              <code>vr_thumbstickFullDeflection 0.85</code> — thumbstick
              response curve endpoints.
            </li>
            <li>
              <code>vr_triggerSensitivity 0.25</code> — how light a trigger pull
              fires the weapon. Higher is more sensitive, so a shorter pull
              fires. Range <code>0.1</code>–<code>0.9</code>.
            </li>
            <li>
              <code>vr_hudScale 1.5</code> — HUD size multiplier in VR.
            </li>
            <li>
              <code>vr_hudDepth 3</code> — how far out the HUD renders in the
              field of view (<code>0</code>–<code>5</code>).
            </li>
          </ul>
          <PlatformOnly platform="quest">
            <p>
              Quest-only: <code>vr_refreshrate 120</code> sets the headset
              display refresh rate; <code>com_maxfps 0</code> uncaps the
              engine's frame rate so it tracks the headset.
            </p>
          </PlatformOnly>
        </div>
      </PlatformOnly>

      <div className="about-section">
        <DocsH2 id="network">Network tuning</DocsH2>
        <ul className="docs-cvars">
          <li>
            <code>cl_maxpackets 125</code> — outgoing packet rate ceiling, in
            packets per second.
          </li>
          <li>
            <code>snaps 40</code> — requested snapshot rate from the server.
          </li>
          <li>
            <code>rate 50000</code> — bandwidth budget for the connection, in
            bytes per second.
          </li>
          <li>
            <code>cl_guidServerUniq 0</code> — keep your <code>qkey</code>
            -derived identity stable across servers. <code>1</code> generates a
            different identity per server. See{" "}
            <Link to="/docs/account">the Account page</Link> for background on
            identity.
          </li>
        </ul>
      </div>
    </>
  );
}
