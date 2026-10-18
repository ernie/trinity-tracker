import { Link } from "react-router-dom";
import { DocsH2 } from "./DocsH2";
import { PlatformOnly } from "./PlatformOnly";
import { PlatformNote } from "./PlatformNote";
import { MODE_PROFILES } from "../ServerCard";

// Each entry states what a setting does, its default and range; the reader
// decides how to tune it.
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
              <code>q3config.cfg</code>:
            </strong>{" "}
            saved on shutdown, with everything you change in the in-game menus.
          </li>
          <li>
            <strong>
              <code>autoexec.cfg</code>:
            </strong>{" "}
            read after <code>q3config.cfg</code> on every launch and never
            overwritten. Put settings here that have no menu row, or that you
            want to keep no matter what changes in the menus.
          </li>
        </ul>
        <PlatformNote platform="vr">
          <p>
            On a PC, VR uses the same install and files as flatscreen. On Quest
            and PICO, both files live at <code>/sdcard/Trinity/baseq3/</code> on
            the headset's internal storage. On a Steam Frame, they live in{" "}
            <code>~/.trinity/baseq3/</code>.
          </p>
        </PlatformNote>
      </div>

      <PlatformOnly platform="vr">
        <div className="about-section">
          <DocsH2 id="vr" platforms={["vr"]}>
            VR comfort &amp; controllers
          </DocsH2>
          <p>
            Controller buttons are bound like keys, in{" "}
            <strong>Setup → VR Options → Bindings</strong>. Each action has a{" "}
            <strong>Button</strong> and an <strong>Alt + Button</strong> column,
            the rows show your controller&rsquo;s own button glyphs, and
            bindings follow your hands: switching handedness or the thumbsticks
            moves the glyphs and keeps the actions where they are. At the
            console the same bindings are{" "}
            <code>
              vrbind &lt;context&gt;[+alt] &lt;key&gt; &lt;command&gt;
            </code>
            .
          </p>
          <p>
            The <strong>VKB</strong> tab holds the bindings that apply while the
            VR keyboard is open: Space, Backspace, Tab, and Enter. Its{" "}
            <code>vrbind</code> context is <code>keyboard</code>.
          </p>
          <p>
            Most VR settings have a row under{" "}
            <strong>Setup → VR Options</strong>: <strong>Controls</strong> for
            aiming, input, and turning, <strong>Comfort</strong> for vignette,
            height, and haptics, and <strong>HUD &amp; Display</strong> for HUD
            placement, display, and rendering. Their cvars, for{" "}
            <code>autoexec.cfg</code>:
          </p>
          <ul className="docs-cvars">
            <li>
              <code>vr_snapturn</code>: <strong>Turning Mode</strong>.{" "}
              <code>0</code> (the default) turns smoothly, <code>1</code> snaps
              by 45 degrees per stick flick, and any higher value snaps by that
              many degrees. The menu offers 45 and 90.
            </li>
            <li>
              <code>vr_directionMode</code>: <strong>Direction Mode</strong>,
              what forward means when you move. <code>0</code> follows your
              head; <code>1</code> (the default) follows your off-hand
              controller.
            </li>
            <li>
              <code>vr_twoHandedWeapons</code>:{" "}
              <strong>Two-Handed Weapons</strong>. Hold the secondary grip to
              stabilize the weapon.
              <ul>
                <li>
                  <code>0</code> off: one-handed aiming
                </li>
                <li>
                  <code>1</code> Basic (the default): the weapon aims along the
                  line from your primary controller toward your secondary
                  controller
                </li>
                <li>
                  <code>2</code> VR Gun Stock: the weapon anchors near your
                  face, like sighting along a stock
                </li>
              </ul>
            </li>
            <li>
              <code>vr_thumbstickDeadzone</code> (default <code>0.15</code>) and{" "}
              <code>vr_thumbstickFullDeflection</code> (default{" "}
              <code>0.85</code>): how far you push the stick before it moves
              you, and how far until it moves you at full speed. These have no
              menu row.
            </li>
            <li>
              <code>vr_triggerSensitivity</code>:{" "}
              <strong>Trigger Sensitivity</strong>, how light a trigger pull
              fires the weapon. Higher is more sensitive, so a shorter pull
              fires. Range <code>0.1</code> to <code>0.9</code>, default{" "}
              <code>0.25</code>.
            </li>
            <li>
              <code>vr_hudScale</code>: <strong>HUD Scale</strong>, a size
              multiplier for the HUD. The menu goes from <code>0.5</code> to{" "}
              <code>2</code>; the default is <code>1</code>.
            </li>
            <li>
              <code>vr_hudDepth</code>: <strong>HUD Depth</strong>, how far away
              the HUD sits, from <code>0</code> to <code>5</code>. Default{" "}
              <code>3</code>.
            </li>
            <li>
              <code>vr_refreshrate</code>: <strong>Refresh Rate</strong>, in Hz.
              The row lists only the rates your headset&rsquo;s runtime reports,
              lowest first, and a value it can&rsquo;t do snaps to the nearest
              one it can. Applies live. Default <code>90</code>.
            </li>
            <li>
              <code>vr_foveation</code> and <code>vr_foveationStrength</code>:{" "}
              <strong>Foveated Rendering</strong> (<code>0</code> Off,{" "}
              <code>1</code> Fixed, <code>2</code> Eye-Tracked, the default) and{" "}
              <strong>Foveation Strength</strong> (<code>1</code> Low,{" "}
              <code>2</code> Medium, the default, <code>3</code> High). See{" "}
              <Link to="/docs/play#vr">VR-specific features</Link>.
            </li>
          </ul>
          <p>
            While VR is running, the headset paces every frame at its own
            refresh rate; <code>com_maxfps</code> applies to flatscreen play.
          </p>
        </div>
      </PlatformOnly>

      <div className="about-section">
        <DocsH2 id="trinity-features">Trinity feature toggles</DocsH2>
        <ul className="docs-cvars">
          <li>
            <code>cg_damagePlums</code>: <strong>Damage Numbers</strong> in Game
            Options, the floating damage numbers above each hit. <code>0</code>{" "}
            hides them; default <code>1</code>.
          </li>
          <li>
            <code>cg_damagePlumScale</code>: a size multiplier for the damage
            numbers. Default <code>1.0</code>; no menu row.
          </li>
          <li>
            <code>cg_hitSounds</code>: <strong>Hit Sounds</strong> in Game
            Options. Default <code>0</code>.
            <ul>
              <li>
                <code>0</code> Classic: the game's own, baseq3's single beep or
                Team Arena's armor tones
              </li>
              <li>
                <code>1</code> Big Hits Low: lower pitch on bigger hits
              </li>
              <li>
                <code>2</code> Big Hits High: higher pitch on bigger hits
              </li>
            </ul>
          </li>
          <li>
            <code>cg_damageEffect</code>: <strong>Damage Effect</strong> in Game
            Options. <code>1</code> (the default) is the directional red
            vignette; <code>0</code> is the classic blood blob.
          </li>
          <li>
            <code>com_blood</code>: <strong>Blood</strong> in Game Options.{" "}
            <code>2</code> (the default) is modern blood that reacts to how hard
            you hit, with spray, lingering trails, and marks on nearby surfaces;{" "}
            <code>1</code> is classic sprite blood; <code>0</code> is off.
          </li>
          <li>
            <code>cg_bloodDensity</code>: <strong>Blood Density</strong> in Game
            Options, how much modern blood is drawn: <code>0</code> Light,{" "}
            <code>1</code> Medium, <code>2</code> Heavy. It&rsquo;s grayed out
            unless Blood is Modern.{" "}
            <PlatformOnly platform="flatscreen">
              Default <code>2</code>.
            </PlatformOnly>
            <PlatformOnly platform="vr">
              Default <code>2</code> on a PC and <code>0</code> on Steam Frame,
              Quest, and PICO.
            </PlatformOnly>
          </li>
          <li>
            <code>cg_drawTimer</code>: <strong>Show Time</strong> in Game
            Options, the match timer in the HUD. <code>0</code> hides it;
            default <code>1</code>.
          </li>
          <li>
            <code>cl_tvDownload</code>: <strong>TVD Download</strong> in Game
            Options, the offer to download the match&rsquo;s TV demo. The prompt
            appears at the start of the next match. Default <code>1</code>.
            <ul>
              <li>
                <code>0</code> Off: no prompt
              </li>
              <li>
                <code>1</code> Decline: prompt, declining if you don&rsquo;t
                answer
              </li>
              <li>
                <code>2</code> Accept: prompt, accepting if you don&rsquo;t
                answer
              </li>
            </ul>
          </li>
          <li>
            <code>cg_followMode</code>: <strong>Follow Camera</strong> in Game
            Options, the camera the follow view starts in when you spectate live
            or watch a TV demo, flatscreen or VR. Switching cameras in-game
            saves the choice here. Default <code>0</code>.
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
            legacy path, for very old hardware). It has no menu row: set it in{" "}
            <code>autoexec.cfg</code>, or run <code>vid_restart</code> after
            changing it. HDR output below is Vulkan-only.
          </p>
        </PlatformOnly>
        <PlatformNote platform="vr">
          <p>
            Steam Frame, Quest and PICO play VR only, with the Vulkan renderer
            only, and have no Display Mode setting. On a PC,{" "}
            <strong>Setup → Display → Display Mode</strong> picks flatscreen or
            VR.
          </p>
        </PlatformNote>
        <PlatformOnly platform="vr">
          <h3>Direct rendering</h3>
          <p>
            <code>r_fbo 0</code> renders straight into the headset with no
            post-processing pass. It&rsquo;s faster, and it costs you bloom
            (and, on a PC, the HDR mirror, screenshots, and overbright). The
            default is <code>1</code>. The <strong>MSAA</strong> row under{" "}
            <strong>Setup → System → Graphics</strong> applies either way.
          </p>
        </PlatformOnly>
        <PlatformOnly platform="flatscreen">
          <h3>Window mode</h3>
          <p>
            <strong>Setup → System → Graphics → Window Mode</strong> picks how
            the game fills your screen:
          </p>
          <ul>
            <li>
              <strong>Borderless</strong> (the default) covers the desktop at
              its own resolution and refresh rate, with no mode switch, so
              alt-tab is instant.
            </li>
            <li>
              <strong>Fullscreen</strong> takes the display exclusively, at the
              Resolution and Refresh Rate you choose.
            </li>
            <li>
              <strong>Windowed</strong> runs in a window at the chosen
              Resolution.
            </li>
          </ul>
          <p>
            Resolution lists what your display reports, grouped by aspect ratio
            with your desktop's shape first. <strong>Custom</strong> takes any
            size: pick a ratio, type the width or height, and the other fills
            in. <kbd>Alt</kbd>+<kbd>Enter</kbd> switches to windowed and back to
            whichever fullscreen kind you used last.
          </p>
        </PlatformOnly>
        <PlatformOnly platform={["flatscreen", "vr"]}>
          <h3>HDR</h3>
          <p>
            On an HDR display, Trinity can output HDR. It needs the Vulkan
            renderer and HDR turned on in your OS display settings, and it is
            off by default.
          </p>
        </PlatformOnly>
        <PlatformOnly platform="flatscreen">
          <p>
            Turn it on under <strong>Setup → Display → HDR Display</strong>,
            then run <code>vid_restart</code>. Calibrate under{" "}
            <strong>Setup → Display → HDR Calibration</strong>: raise{" "}
            <strong>Peak</strong> until the inner rectangle&rsquo;s edge just
            disappears into the outer one; that&rsquo;s your display&rsquo;s
            real peak brightness. A Mac has no calibration screen and needs
            none. <strong>Setup → Display → Highlight Boost</strong> sets how
            bright the brightest highlights get. <code>r_hdrActive</code> reads{" "}
            <code>1</code> while HDR output is on.
          </p>
          <p>
            <strong>SDR Gamma</strong>, right under HDR Display, sets how the
            menus and the rest of the standard-range picture look in HDR.{" "}
            <strong>2.2</strong> (the default) matches how the game looks on an
            SDR monitor; <strong>sRGB</strong> matches how Windows shows SDR
            apps in HDR, with lifted darks and slightly washed-out color.
          </p>
          <p>
            Prefer the config file? Set <code>r_hdrDisplay 1</code> and{" "}
            <code>r_hdrPeak</code> (your calibrated peak in nits) in{" "}
            <code>autoexec.cfg</code>. Paper-white is automatic by default;{" "}
            <code>r_hdrSaturation</code>, <code>r_hdrSaturationFull</code>, and{" "}
            <code>r_hdrSoftKnee</code> are advanced settings covered in{" "}
            <Link to="/docs/reference">Reference</Link>.
          </p>
        </PlatformOnly>
        <PlatformOnly platform="vr">
          <p>
            Headsets render standard range. In VR, HDR output applies only to
            the <strong>desktop mirror window</strong> on a PC, with the Vulkan
            renderer. Steam Frame, Quest and PICO have no HDR output.
          </p>
          <p>
            For the mirror on a PC, turn it on under{" "}
            <strong>Setup → Display → HDR Display</strong>, then run{" "}
            <code>vid_restart</code>.
          </p>
          <p>
            A peak calibrated in flatscreen applies unchanged. The calibration
            screen is here too: in{" "}
            <strong>Setup → Display → HDR Calibration</strong>, raise{" "}
            <strong>Peak</strong> until the inner rectangle&rsquo;s edge just
            disappears into the outer one. The mirror isn't HDR as the headset
            renders it, so slide the headset up and read the pattern off the
            monitor.
          </p>
          <p>
            Prefer the config file? Set <code>r_hdrDisplay 1</code> and{" "}
            <code>r_hdrPeak</code> (your calibrated peak in nits) in{" "}
            <code>autoexec.cfg</code>.
          </p>
        </PlatformOnly>
      </div>

      <div className="about-section">
        <DocsH2 id="voice-chat">Voice chat</DocsH2>
        <p>
          The voice chat rows are in <strong>Setup → System → Sound</strong>.
          When voice isn&rsquo;t working, see{" "}
          <Link to="/docs/install#troubleshooting">Troubleshooting</Link> on the
          Install page.
        </p>
        <ul className="docs-cvars">
          <li>
            <code>cl_voip</code>: <strong>VOIP</strong>, voice chat on (
            <code>1</code>, the default) or off.
          </li>
          <li>
            <code>cl_voipVolume</code>: <strong>VOIP Volume</strong>, how loud
            incoming voice plays, separate from the main and music volumes.{" "}
            <code>0</code> to <code>2</code>, default <code>1</code>.
          </li>
          <li>
            <code>cl_voipUseVAD</code>: <strong>Activation</strong>.
            <ul>
              <li>
                <code>0</code> Push to talk (the default): hold{" "}
                <strong>VoIP PTT / Toggle</strong> to talk
              </li>
              <li>
                <code>1</code> When speaking: transmits when the mic picks up
                sound above the Threshold
              </li>
            </ul>
          </li>
          <li>
            <code>cl_voipVADThreshold</code>: <strong>Threshold</strong>, the
            mic level that counts as speaking with When speaking activation.{" "}
            <code>0</code> to <code>1</code>, default <code>0.1</code>.
          </li>
          <li>
            <code>cl_voipGainDuringCapture</code>:{" "}
            <strong>Transmit Game Volume</strong>, the game&rsquo;s volume while
            you transmit, from <code>0.2</code> to <code>1</code> (the default).
            Lower values keep more game audio out of your mic.
          </li>
          <li>
            <code>cg_drawVoipSpeakers</code>: the list of players speaking right
            now in the upper right of the HUD, with channel-colored speaker
            icons. Default <code>1</code>; no menu row.
          </li>
          <li>
            <code>cl_voipMuteAll</code>, <code>cl_voipMuteSpatial</code>,{" "}
            <code>cl_voipMuteDirect</code>, and <code>cl_voipMuteTeam</code>:
            mute incoming voice per channel. No menu row.
          </li>
        </ul>
        <p>
          The two voice bindings are in <strong>Setup → Controls</strong>:
        </p>
        <ul className="docs-cvars">
          <li>
            <code>+voiprecord</code>: <strong>VoIP PTT / Toggle</strong>. Hold
            to talk with Push to talk; tap to mute or unmute with When speaking.
          </li>
          <li>
            <code>voiptarget [spatial|team|all]</code>:{" "}
            <strong>Voice Target</strong>. With no argument it cycles through
            the three channels; with one it sets <code>cl_voipSendTarget</code>{" "}
            directly.
          </li>
        </ul>
      </div>

      <div className="about-section">
        <DocsH2 id="modes-note">Game modes</DocsH2>
        <p>
          Trinity has four game modes, each setting movement physics and combat
          rules: Quake III Arena (<code>g_mode 0</code>), CPMA (<code>1</code>),
          Quake Live (<code>2</code>), and Quake Live Turbo (<code>3</code>).
          For your own games, pick one with the <strong>Mode</strong> selector
          in the create-server and single-player menus, or set{" "}
          <code>g_mode</code> at the console and run <code>map_restart</code>.
        </p>
        <p>
          In multiplayer the server picks the mode. The server browser shows
          each server&rsquo;s mode as an icon to the left of its name; in a
          match, pull up the scoreboard to see it as a small icon in the top
          right of the HUD.
        </p>
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
          Override what other players' models render as. Skins suffixed with{" "}
          <code>/pm</code> (promode) or <code>/fb</code> (fullbright) accept the
          color overrides; stock skins ignore them.
        </p>
        <ul className="docs-cvars">
          <li>
            <code>cg_enemyModel "keel/pm"</code>: every enemy renders as this
            model and skin. Empty (the default) turns it off.
          </li>
          <li>
            <code>cg_enemyColors "333"</code>: a color string of up to five
            digits for head, body, legs, <code>color1</code> (rail core and
            weapon glow), and <code>color2</code> (rail spiral). Three digits
            set only the model tints, and the enemy keeps their own rail colors.{" "}
            <a href="#color-codes">Color codes</a> below has the digit-to-color
            table.
          </li>
          <li>
            <code>cg_teamModel "doom/pm"</code>: the same as{" "}
            <code>cg_enemyModel</code>, for teammates.
          </li>
          <li>
            <code>cg_teamColors "222"</code>: the same color string, for
            teammates.
          </li>
          <li>
            <code>cg_deadBodyDarken 1</code>: gray out dead bodies (requires pm
            or fb skins). Default <code>1</code>.
          </li>
        </ul>
      </div>

      <div className="about-section">
        <DocsH2 id="color-codes">Color codes</DocsH2>
        <p>
          Three things in Quake III use single-digit color codes, and each has
          its own: the color slider in the player-settings menu, the{" "}
          <code>color1</code> / <code>color2</code> cvars (and the forced-model
          colors built on them), and the <code>^N</code> escapes in chat and
          player names. They disagree about which digit means which color.
        </p>

        <h3>The player-settings color slider</h3>
        <p>
          The slider's swatches store these <code>color1</code> digits:
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
          The digits matter only when hand-editing <code>color1</code>/
          <code>color2</code> or <code>cg_enemyColors</code>/
          <code>cg_teamColors</code>.
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
        <p>
          By default, Trinity servers give everyone on red or blue in a team
          game their team&rsquo;s <code>color1</code> (<code>4</code> on red,{" "}
          <code>1</code> on blue), while <code>color2</code> stays each
          player&rsquo;s own, and give each stock bot its own{" "}
          <code>color1</code> / <code>color2</code> pair. Server operators can
          switch these off with <code>g_forceTeamColors</code> and{" "}
          <code>g_botColors</code>. Either way, the fourth and fifth digits of{" "}
          <code>cg_teamColors</code> recolor your teammates on your screen.
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
          for the player&rsquo;s team color: <code>4</code> on red,{" "}
          <code>1</code> on blue, <code>7</code> in FFA. One config string then
          follows the teams from match to match.
        </p>

        <h3>
          Chat / name <code>^N</code> color escapes
        </h3>
        <p>
          In text (chat, player names, the frag feed, the scoreboard, and
          menus), escapes like <code>^1text</code> use a different mapping:
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
          differs; most painfully, <code>1</code> and <code>4</code> swap. Chat{" "}
          <code>^1</code> is red, but <code>color1 1</code> is blue. Chat{" "}
          <code>^4</code> is blue, but <code>color1 4</code> is red.
        </p>
        <p>
          For names and chat that look the same to players on every engine, stay
          within <code>^0</code> through <code>^7</code>.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="network">Network tuning</DocsH2>
        <p>
          The snapshot rate always follows the server's <code>sv_fps</code>;
          clients have no <code>snaps</code> setting.
        </p>
        <ul className="docs-cvars">
          <li>
            <code>cl_packetdup</code>: <strong>Packet Recovery</strong>, the one
            row in <strong>Setup → Network</strong>. <strong>Low</strong> (
            <code>1</code>, the default), <strong>Medium</strong> (
            <code>2</code>), or <strong>High</strong> (<code>3</code>). Low
            rides out one lost packet in a row, Medium two, and High three; each
            step up costs a little more upload.
          </li>
          <li>
            <code>rate</code>: the connection&rsquo;s bandwidth budget, in bytes
            per second. Default <code>50000</code>; no menu row.
          </li>
          <li>
            <code>cl_maxpackets</code>: the most packets you send per second,
            from <code>15</code> to <code>125</code> (the default). No menu row.
          </li>
          <li>
            <code>cl_guidServerUniq</code>: <code>0</code> (the default) keeps
            the identity built from your <code>qkey</code> the same on every
            server; <code>1</code> gives you a different identity per server.{" "}
            <Link to="/docs/account">Account</Link> explains identity.
          </li>
        </ul>
      </div>
    </>
  );
}
