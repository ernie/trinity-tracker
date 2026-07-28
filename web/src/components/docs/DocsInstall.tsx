import { Link } from "react-router-dom";
import { useGitHubReleases } from "../../hooks/useGitHubReleases";
import { DISCORD_INVITE_URL } from "../../constants/discord";
import { ExternalLinkIcon } from "../ExternalLinkIcon";
import { DocsH2 } from "./DocsH2";
import { PlatformTabs } from "./PlatformTabs";
import { PlatformNote } from "./PlatformNote";
import { PlatformOnly } from "./PlatformOnly";
import { PlatformChip } from "./PlatformChip";

// Per-OS direct-download mapping for the Flatscreen panel. macOS
// ships a universal2 binary (Intel + Apple Silicon in one); Windows
// and Linux are x64 only. Fall through to the releases page for
// anything else.
type DetectedOS = "windows" | "macos" | "linux";

const FLATSCREEN_DOWNLOADS: Record<
  DetectedOS,
  { asset: string; label: string }
> = {
  windows: {
    asset: "trinity-windows-mingw-x86_64.zip",
    label: "Windows (x64)",
  },
  macos: { asset: "trinity-macos-universal2.dmg", label: "macOS" },
  linux: { asset: "trinity-linux-x86_64.zip", label: "Linux (x64)" },
};

// PCVR has no macOS build — Apple deprecated SteamVR years ago, so
// PCVR on Mac isn't a viable target. macOS users (and anyone else
// whose OS we don't detect) fall through to the "see all builds"
// link. Partial<> reflects the absent macos key.
const PCVR_DOWNLOADS: Partial<
  Record<DetectedOS, { asset: string; label: string }>
> = {
  // trinityvr-* are the canonical archives; the q3vr-* ones are a
  // transitional auto-update bridge slated for removal.
  windows: {
    asset: "trinityvr-windows-msvc-x86_64.zip",
    label: "Windows (x64)",
  },
  linux: { asset: "trinityvr-linux-x86_64.zip", label: "Linux (x64)" },
};

// Coarse desktop-OS detection from the UA string. Returns null on
// mobile or anything we don't recognize — those users get the "see
// all builds" link instead. Mobile checks come first because Android
// contains "Linux" in its UA and iOS contains "Mac OS X".
function detectOS(): DetectedOS | null {
  if (typeof navigator === "undefined") return null;
  const ua = navigator.userAgent;
  if (/Android/i.test(ua)) return null;
  if (/iPhone|iPad|iPod/i.test(ua)) return null;
  if (/Windows/i.test(ua)) return "windows";
  if (/Mac OS X|Macintosh/i.test(ua)) return "macos";
  if (/Linux|X11/i.test(ua)) return "linux";
  return null;
}

// /docs/install — three-step install guide (download, copy assets,
// install patches), followed by reference sections on Automatic
// Updates and Troubleshooting. Each engine's binary is shown via
// PlatformTabs; small per-platform deviations use PlatformNote.
export function DocsInstall() {
  const { releases } = useGitHubReleases();
  const detectedOS = detectOS();
  const pcvrDownload = detectedOS ? PCVR_DOWNLOADS[detectedOS] : undefined;

  // Map releases to platform — release.repo identifies which engine
  // a binary is for. trinity-engine = flatscreen, trinity-vr = pcvr,
  // trinity-quest = quest. The fourth ('trinity') is the mod itself,
  // bundled with each engine, not a standalone download here.
  const flatscreen = releases.find((r) => r.repo === "trinity-engine");
  const pcvr = releases.find((r) => r.repo === "trinity-vr");
  const quest = releases.find((r) => r.repo === "trinity-quest");

  return (
    <>
      <div className="about-section">
        <DocsH2 id="install-trinity">Step 1 — Download Trinity</DocsH2>
        <p>
          Trinity ships as a custom Quake 3 engine plus the gameplay mod. Each
          engine targets a different way of playing — pick the one that matches
          your setup.
        </p>

        <PlatformTabs>
          <PlatformTabs.Panel platform="flatscreen">
            <p>
              <strong>Trinity Engine</strong> is a Quake3e-based build for
              desktop monitors with keyboard + mouse.
            </p>
            {detectedOS && (
              <a
                href={`https://github.com/ernie/trinity-engine/releases/latest/download/${FLATSCREEN_DOWNLOADS[detectedOS].asset}`}
                className="install-download-link"
              >
                Download Trinity Engine
                {flatscreen?.version ? ` ${flatscreen.version}` : ""} for{" "}
                {FLATSCREEN_DOWNLOADS[detectedOS].label} →
              </a>
            )}
            <p className="install-download-others">
              {detectedOS
                ? "Need a different platform? "
                : "Pick the build for your platform: "}
              <a
                href={
                  flatscreen?.url ??
                  "https://github.com/ernie/trinity-engine/releases/latest"
                }
                target="_blank"
                rel="noopener noreferrer"
              >
                See all builds on the releases page →
              </a>
            </p>
            <p>After downloading:</p>
            {detectedOS === "macos" ? (
              <ol>
                <li>
                  Open the <code>.dmg</code> disk image.
                </li>
                <li>
                  Drag <strong>Trinity</strong> into your{" "}
                  <strong>Applications</strong> folder (or wherever you keep
                  your games).
                </li>
                <li>
                  Launch Trinity once — it creates its data folder at{" "}
                  <code>~/Library/Application&nbsp;Support/Trinity/</code>. That
                  folder is your install — the rest of this guide refers to it
                  as "your install."
                </li>
              </ol>
            ) : (
              <ol>
                <li>Extract the zip.</li>
                <li>
                  Rename the extracted folder to <code>Trinity</code>.
                </li>
                <li>
                  Place it wherever you keep your installed games. That folder
                  is your install — the rest of this guide refers to it as "your
                  install."
                </li>
              </ol>
            )}
            <p>
              Auto-updates are handled by the installed app — see the Automatic
              Updates section below.
            </p>
          </PlatformTabs.Panel>

          <PlatformTabs.Panel platform="pcvr">
            <p>
              <strong>Trinity VR</strong> is a PCVR build based on RippeR37's
              Q3VR. Runs through SteamVR on any PCVR headset (Index, Vive, Rift,
              Quest connected to a PC, etc.).
            </p>
            {pcvrDownload && (
              <a
                href={`https://github.com/ernie/trinity-vr/releases/latest/download/${pcvrDownload.asset}`}
                className="install-download-link"
              >
                Download Trinity VR
                {pcvr?.version ? ` ${pcvr.version}` : ""} for{" "}
                {pcvrDownload.label} →
              </a>
            )}
            <p className="install-download-others">
              {pcvrDownload
                ? "Need a different platform? "
                : "Pick the build for your platform: "}
              <a
                href={
                  pcvr?.url ??
                  "https://github.com/ernie/trinity-vr/releases/latest"
                }
                target="_blank"
                rel="noopener noreferrer"
              >
                See all builds on the releases page →
              </a>
            </p>
            <p>After downloading:</p>
            <ol>
              <li>Extract the zip.</li>
              <li>
                Rename the extracted folder to <code>Trinity VR</code>.
              </li>
              <li>
                Place it wherever you keep your installed games. That folder is
                your install — the rest of this guide refers to it as "your
                install."
              </li>
            </ol>
            <p>
              Auto-updates are handled by the installed app — see the Automatic
              Updates section below.
            </p>
            <PlatformNote platform="pcvr">
              <p>
                <strong>Coming from Q3VR or an older Trinity VR?</strong> Check
                your install's <code>baseq3</code> folder for{" "}
                <code>pakQ3VR.pk3</code> and delete it. Its contents ship in the
                Trinity mod paks now, and a leftover copy overrides them. The
                updater only adds and replaces files — it never deletes — so a
                copy from an older install sticks around on its own.
              </p>
            </PlatformNote>
          </PlatformTabs.Panel>

          <PlatformTabs.Panel platform="quest">
            <p>
              <strong>Trinity Quest</strong> runs natively on Meta Quest 2, 3,
              and 3S — no PC required. Built on Team Beef's Quake3Quest port.
            </p>
            {quest && (
              <a
                href={quest.assetUrl ?? quest.url}
                target="_blank"
                rel="noopener noreferrer"
                className="install-download-link"
              >
                Download Trinity Quest{quest.version ? ` ${quest.version}` : ""}{" "}
                (.apk) →
              </a>
            )}
            <p>
              Sideload the <code>.apk</code> using{" "}
              <a
                href="https://sidequestvr.com/"
                target="_blank"
                rel="noopener noreferrer"
              >
                SideQuest
              </a>{" "}
              — the same tool people use for the standard Team Beef ports.
            </p>
            <PlatformNote platform="quest">
              <p>
                <strong>
                  Already have the Team Beef Quake3Quest installed?
                </strong>{" "}
                Uninstall it before installing Trinity's build — the two can't
                coexist on the same headset. While you're uninstalling, take the
                opportunity to clear any <code>autoexec.cfg</code> and{" "}
                <code>q3config.cfg</code> files from{" "}
                <code>/sdcard/ioquake3Quest/baseq3/</code> and{" "}
                <code>/sdcard/ioquake3Quest/missionpack/</code> so Trinity
                starts with fresh settings.
              </p>
            </PlatformNote>
          </PlatformTabs.Panel>
        </PlatformTabs>
      </div>

      <div className="about-section">
        <DocsH2 id="copy-pak0">Step 2 — Copy your Quake 3 game assets</DocsH2>
        <p>
          Trinity is a mod, not a replacement game — you need a legitimate copy
          of Quake 3 Arena (which includes the Team Arena mission pack) to play.
          The base <code>pak0.pk3</code> files contain the original game assets
          and aren't redistributable, so you bring your own:
        </p>
        <ul>
          <li>
            Buy{" "}
            <a
              href="https://store.steampowered.com/app/2200/Quake_III_Arena/"
              target="_blank"
              rel="noopener noreferrer"
            >
              Quake 3 Arena on Steam
            </a>{" "}
            (or use a retail CD install). Steam's release ships both the base
            game and Team Arena.
          </li>
          <li>
            Find your Quake 3 install folder. In Steam, right-click the game in
            your library → <strong>Manage</strong> →{" "}
            <strong>Browse local files</strong> to open it in your file manager.
          </li>
          <li>
            Copy <code>baseq3/pak0.pk3</code> into your install's{" "}
            <code>baseq3</code> folder (this is the base Quake 3 content).
          </li>
          <li>
            Copy <code>missionpack/pak0.pk3</code> into your install's{" "}
            <code>missionpack</code> folder (this is the Team Arena content —
            needed if you want to play Team Arena modes).
          </li>
        </ul>
        <PlatformNote platform="quest">
          <p>
            On Quest, Trinity's asset folders live at{" "}
            <code>/sdcard/ioquake3Quest/baseq3/</code> and{" "}
            <code>/sdcard/ioquake3Quest/missionpack/</code> on the headset's
            internal storage. The engine creates these directories on first
            launch. Use SideQuest's file browser to copy each{" "}
            <code>pak0.pk3</code> into the matching folder.
          </p>
        </PlatformNote>
        {detectedOS === "macos" && (
          <PlatformNote platform="flatscreen">
            <p>
              On macOS, "your install" is the{" "}
              <code>~/Library/Application&nbsp;Support/Trinity/</code> folder
              from Step 1 — copy each <code>pak0.pk3</code> into the matching
              subfolder there.
            </p>
          </PlatformNote>
        )}
      </div>

      <div className="about-section">
        <DocsH2 id="install-patches">
          Step 3 — Install the 1.32 point-release patches
        </DocsH2>
        <p>
          The Quake 3 1.32 point release ships{" "}
          <strong>
            required patches for both Quake III Arena and Team Arena
          </strong>
          .
        </p>
        <p>
          The patches ship under id Software's EULA, so we gate the download
          behind a quick read-and-accept page:{" "}
          <Link to="/quake3-eula" target="_blank" rel="noopener noreferrer">
            Read the EULA and download the 1.32 patches
            <ExternalLinkIcon className="external-link-icon" />
          </Link>
          .
        </p>
        <p>
          Copy each folder's files into the matching folder in your install —{" "}
          <code>baseq3</code> files into <code>baseq3</code>,{" "}
          <code>missionpack</code> files into <code>missionpack</code>.
        </p>
        <div className="install-complete">
          <p className="install-complete__title">
            That's it — Trinity is installed and ready to play.
          </p>
          <p>Have fun! We'll see you in the arena.</p>
        </div>
      </div>

      <div className="about-section">
        <DocsH2 id="automatic-updates">Automatic Updates</DocsH2>
        <PlatformOnly platform={["flatscreen", "pcvr"]}>
          <p>
            Trinity checks for new releases on startup. When an update is
            available, an indicator appears on the main menu — download and
            apply it from there. The engine handles the relaunch automatically.
          </p>
          <PlatformNote platform="pcvr">
            <p>
              Updates add and replace files; they never delete. If your install
              dates back to Q3VR or an early Trinity VR, delete{" "}
              <code>baseq3/pakQ3VR.pk3</code> by hand — the assets it used to
              carry ship in the Trinity mod paks now, and the stale copy
              overrides them.
            </p>
          </PlatformNote>
        </PlatformOnly>
        <PlatformOnly platform="quest">
          <p>
            Trinity checks for new releases on startup. When an update is
            available, an indicator appears on the main menu — download and
            apply it from there. After Android finishes installing the new APK,
            relaunch Trinity from the headset's app library.
          </p>
        </PlatformOnly>
      </div>

      <div className="about-section">
        <DocsH2 id="troubleshooting">Troubleshooting</DocsH2>
        <p>The most common install issues, with what to check first.</p>
        <div className="install-troubleshooting">
          <PlatformOnly platform="flatscreen">
            <details className="install-trouble">
              <summary>
                Trinity won't launch — "you need to install Quake III Arena"
                <PlatformChip platform="flatscreen" />
              </summary>
              <div className="install-trouble__body">
                <p>
                  Trinity needs <code>pak0.pk3</code> from a legitimate Quake 3
                  install. If it's missing or corrupt, Trinity quits with a
                  fatal-error dialog ending in{" "}
                  <em>
                    "you need to install Quake III Arena in order to play"
                  </em>
                  ; the console (or log) spells out the cause — look for{" "}
                  <em>"pak0.pk3 is missing"</em> or{" "}
                  <em>"Point Release files are missing"</em>.
                </p>
                <p>
                  Re-check Step 2 (and Step 3 for the point-release patches).{" "}
                  <code>pak0.pk3</code> should sit in your install's{" "}
                  <code>baseq3</code> folder, and{" "}
                  <code>missionpack/pak0.pk3</code> too if you want Team Arena.
                  On case-sensitive filesystems the filename has to be lowercase
                  exactly.
                </p>
              </div>
            </details>
          </PlatformOnly>

          <PlatformOnly platform="pcvr">
            <details className="install-trouble">
              <summary>
                Trinity won't launch — "Quake 3 data files are missing"
                <PlatformChip platform="pcvr" />
              </summary>
              <div className="install-trouble__body">
                <p>
                  Trinity needs <code>pak0.pk3</code> from a legitimate Quake 3
                  install. If files are missing, Trinity quits with a fatal
                  error like{" "}
                  <em>"Quake 3 data files are missing. Please copy …"</em>{" "}
                  followed by a list of the specific paks it couldn't find. Team
                  Arena content surfaces the same way —{" "}
                  <em>"Quake 3 Team Arena data files are missing"</em>.
                </p>
                <p>
                  Re-check Step 2 (and Step 3 for the point-release patches).{" "}
                  <code>pak0.pk3</code> should sit in your install's{" "}
                  <code>baseq3</code> folder, and{" "}
                  <code>missionpack/pak0.pk3</code> too if you want Team Arena.
                  On case-sensitive filesystems the filename has to be lowercase
                  exactly.
                </p>
              </div>
            </details>
          </PlatformOnly>

          <PlatformOnly platform="quest">
            <details className="install-trouble">
              <summary>
                Trinity won't launch — "pak0.pk3" is missing
                <PlatformChip platform="quest" />
              </summary>
              <div className="install-trouble__body">
                <p>
                  Trinity needs <code>pak0.pk3</code> from a legitimate Quake 3
                  install. If it's missing, Trinity quits with a fatal error
                  including{" "}
                  <em>
                    "pak0.pk3" is missing. Please copy it from your legitimate
                    Q3 CDROM.
                  </em>{" "}
                  If the patches are missing too, you'll also see{" "}
                  <em>
                    "Point Release files are missing. Please re-install the 1.32
                    point release."
                  </em>
                </p>
                <p>
                  Re-check Step 2 (and Step 3 for the point-release patches).{" "}
                  <code>pak0.pk3</code> should sit in{" "}
                  <code>/sdcard/ioquake3Quest/baseq3/</code> on the headset, and{" "}
                  <code>/sdcard/ioquake3Quest/missionpack/pak0.pk3</code> too if
                  you want Team Arena.
                </p>
              </div>
            </details>
          </PlatformOnly>

          <PlatformOnly platform={["flatscreen", "pcvr"]}>
            <details className="install-trouble">
              <summary>
                A security popup appears when I launch
                <PlatformChip platform={["flatscreen", "pcvr"]} />
              </summary>
              <div className="install-trouble__body">
                <ul>
                  <li>
                    <strong>Windows:</strong> SmartScreen may flag the unsigned
                    binary on first run. Click "More info" → "Run anyway."
                  </li>
                  <li>
                    <strong>macOS:</strong> a dialog will confirm the app was
                    downloaded from the internet. Click "Open" to continue.
                  </li>
                </ul>
                <p>Subsequent launches won't prompt.</p>
              </div>
            </details>
          </PlatformOnly>

          <PlatformOnly platform="quest">
            <details className="install-trouble">
              <summary>
                APK won't install
                <PlatformChip platform="quest" />
              </summary>
              <div className="install-trouble__body">
                <p>
                  The most common cause of this is the older Team Beef release
                  being on your headset. The updated version is signed by a
                  different author and can't install over top of the Team Beef
                  release. Uninstall the Team Beef Quake3Quest through Quest's
                  app library, then install Trinity's build. While you're at it,
                  clear any <code>autoexec.cfg</code> and{" "}
                  <code>q3config.cfg</code> files from{" "}
                  <code>/sdcard/ioquake3Quest/baseq3/</code> and{" "}
                  <code>/sdcard/ioquake3Quest/missionpack/</code> so Trinity
                  starts with fresh settings.
                </p>
              </div>
            </details>
          </PlatformOnly>

          <details className="install-trouble">
            <summary>
              Update isn't being detected, or you're stuck on an older version
            </summary>
            <div className="install-trouble__body">
              <p>
                If you aren't receiving updates, you can force a re-check from
                the in-game console:
              </p>
              <ol>
                <li>
                  Open the console.{" "}
                  <PlatformOnly platform="flatscreen">
                    Default key is <code>~</code>.
                  </PlatformOnly>
                  <PlatformOnly platform={["pcvr", "quest"]}>
                    In VR, open it from the in-game menu — there's no keyboard{" "}
                    <code>~</code> binding in VR.
                  </PlatformOnly>
                </li>
                <li>
                  Run <code>update_force 1</code> — this bypasses the version
                  comparison so the engine treats the latest release as
                  available even if your version looks current.
                </li>
                <li>
                  Run <code>update</code> — kicks off a fresh check against
                  GitHub. When it finishes, the main menu's update indicator
                  lights up.
                </li>
                <li>Apply from the menu as usual.</li>
              </ol>
              <p>
                <code>update_force</code> is session-only — it resets next
                launch, so you don't need to clear it.
              </p>
            </div>
          </details>

          <details className="install-trouble">
            <summary>My stats aren't showing up on this site</summary>
            <div className="install-trouble__body">
              <p>
                Stats only flow from servers that run the Trinity collector.
                Trinity is backwards-compatible with vanilla servers, but
                vanilla servers don't report matches anywhere. Check the server
                cards on the <Link to="/servers">Servers page</Link> —
                collector-enabled servers are labeled.
              </p>
              <p>
                You don't need a Trinity account for the hub to track you —
                accounts come into play if your stats end up split across
                multiple installs (next item).
              </p>
            </div>
          </details>

          <details className="install-trouble">
            <summary>
              My stats look too low / I see more than one player with my name
            </summary>
            <div className="install-trouble__body">
              <p>
                Trinity identifies you by a per-install file (your{" "}
                <code>qkey</code>), so each Trinity install starts out as a
                separate identity from the hub's perspective. If you've
                installed on more than one machine — or reinstalled — your stats
                can end up split across two or three "yous," even if the in-game
                name is the same.
              </p>
              <p>
                Sign into a Trinity account on every platform you play on, and
                the hub merges your installs automatically — no manual{" "}
                <code>!link</code> commands needed. See the{" "}
                <Link to="/docs/account">Account docs</Link> for the sign-in
                flow.
              </p>
            </div>
          </details>

          <details className="install-trouble">
            <summary>I can't hear other players on voice chat</summary>
            <div className="install-trouble__body">
              <p>A few things to check, in order:</p>
              <ul>
                <li>
                  <strong>Are you receiving voice at all?</strong> When someone
                  speaks, a speaker icon shows up over their head and in the
                  talker stack at the upper right of the HUD. If those never
                  appear, you're not getting voice from the server.
                </li>
                <li>
                  <strong>Voice volume.</strong> The game has a separate
                  voice-chat volume from main and music volume — find the voice
                  slider in the in-game audio settings. If your other volumes
                  are loud, voice can be drowned out without you realizing it's
                  just turned down.
                </li>
                <li>
                  <strong>Client setting.</strong> <code>cl_voip 1</code> must
                  be set in your config (it's the default, so this is usually
                  fine).
                </li>
                <li>
                  <strong>Server setting.</strong> If the indicators never
                  appear no matter who's playing, the server probably doesn't
                  have voice chat turned on. Try a different server, or ask in
                  the <a href={DISCORD_INVITE_URL}>Trinity Discord</a> for
                  server-specific help.
                </li>
              </ul>
            </div>
          </details>

          <details className="install-trouble">
            <summary>People can't hear me on voice chat</summary>
            <div className="install-trouble__body">
              <p>
                Look at the speaker icon on your own player portrait in the HUD
                — that's the fastest way to read your transmit state:
              </p>
              <ul>
                <li>
                  <strong>Speaker with an X next to it.</strong> You're
                  self-muted. Unmute and try again.
                </li>
                <li>
                  <strong>Speaker with no sound waves.</strong> Your mic is on
                  but you're not currently transmitting. Depending on your voice
                  settings, you may need to press a bound input — push-to-talk
                  requires holding a key while you speak; voice-activity mode
                  transmits automatically when it picks up sound.
                </li>
                <li>
                  <strong>Speaker with sound waves.</strong> You're
                  transmitting. If people still can't hear you, the issue is
                  downstream — server config or their end. Hop into the{" "}
                  <a href={DISCORD_INVITE_URL}>Trinity Discord</a> for help
                  digging in.
                </li>
              </ul>
            </div>
          </details>
        </div>
      </div>
    </>
  );
}
