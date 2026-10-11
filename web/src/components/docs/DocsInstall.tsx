import { Link } from "react-router-dom";
import { useGitHubReleases } from "../../hooks/useGitHubReleases";
import { useInstallerDownload } from "../../hooks/useInstallerDownload";
import { DISCORD_INVITE_URL } from "../../constants/discord";
import { DocsH2 } from "./DocsH2";
import { PlatformNote } from "./PlatformNote";

// /docs/install — get Quake 3 and run the Trinity Installer; then
// Automatic Updates and Troubleshooting. Flat and VR share
// one install; VR-only details (headsets) sit in PlatformNotes.
export function DocsInstall() {
  const { releases } = useGitHubReleases();
  const installer = useInstallerDownload();
  const installerRelease = releases.find((r) => r.repo === "trinity-installer");

  return (
    <>
      <div className="about-section">
        <DocsH2 id="get-quake3">Step 1 — Get Quake III Arena</DocsH2>
        <p>
          Trinity needs a retail Quake III Arena with Team Arena; it uses their{" "}
          <code>pak0.pk3</code> files.
        </p>
        <p>
          Buy{" "}
          <a
            href="https://store.steampowered.com/app/2200/Quake_III_Arena/"
            target="_blank"
            rel="noopener noreferrer"
          >
            Quake 3 Arena on Steam
          </a>{" "}
          (or use a retail CD install) and install it on a PC or Mac. Steam's
          release ships both the base game and Team Arena. The Trinity Installer
          copies the game files from there, wherever you're installing Trinity.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="install-trinity">Step 2 — Run the Trinity Installer</DocsH2>
        <p>
          The <strong>Trinity Installer</strong> downloads the latest Trinity
          for the device you pick, copies your Quake 3 game files (fetching the
          1.32 patch files if your install lacks them, after you accept id
          Software's license), and adds Steam shortcuts and SteamVR settings
          where Steam is present.
        </p>
        {installer.direct && (
          <a href={installer.href} className="install-download-link">
            Download the Trinity Installer
            {installer.version ? ` ${installer.version}` : ""} for{" "}
            {installer.platform} →
          </a>
        )}
        <p className="install-download-others">
          {installer.direct
            ? "Need a different platform? "
            : "Pick the installer for your platform: "}
          <a
            href={
              installerRelease?.url ??
              "https://github.com/ernie/trinity-installer/releases/latest"
            }
            target="_blank"
            rel="noopener noreferrer"
          >
            See all builds on the releases page →
          </a>
        </p>
        <p>
          Run it on the computer with Quake 3 installed and choose where Trinity
          goes:
        </p>
        <ul>
          <li>
            <strong>This PC or Mac</strong> — nothing else to prepare. On
            Windows it installs to <code>C:\Games\Trinity</code> by default; on
            Windows and Linux you pick whether shortcuts start in VR or flat,
            and "Add to Steam" adds Trinity to your Steam library (the VR
            shortcut also shows up in SteamVR).
          </li>
          <li>
            <strong>Steam Frame</strong> — turn on Developer Mode on the
            headset, then pair it from{" "}
            <strong>Settings → Developer → Pair new host</strong> when the
            installer asks.
          </li>
          <li>
            <strong>Meta Quest or PICO</strong> — turn on Developer Mode and USB
            debugging, connect the headset over USB, and accept the debugging
            prompt in the headset.
          </li>
        </ul>
        <PlatformNote platform="vr">
          <p>
            On a PC, one Trinity install plays both flat and in VR, through
            SteamVR or any OpenXR runtime; pick one at{" "}
            <strong>Setup → Display → Display Mode</strong>. On a Steam Frame,
            Quest, or PICO, Trinity runs on the headset itself — the computer is
            only needed to install it — and plays VR only, with the Vulkan
            renderer and no Display Mode setting.
          </p>
        </PlatformNote>
      </div>

      <div className="about-section">
        <DocsH2 id="automatic-updates">Automatic Updates</DocsH2>
        <p>
          Trinity checks for new releases on startup. When an update is
          available, an indicator appears on the main menu — download and apply
          it from there. On a PC, Mac, or Steam Frame the engine relaunches
          itself.
        </p>
        <PlatformNote platform="vr">
          <p>
            On Quest and PICO the update installs as a normal app update, so
            your icon and files stay put. After Android finishes installing it,
            relaunch Trinity from the headset's app library.
          </p>
          <p>
            An existing Quest or PICO install moves onto the Trinity Engine
            build with its next in-app update. The app, its icon, and the{" "}
            <code>/sdcard/Trinity</code> folder (configs and <code>qkey</code>)
            stay; old cvars in your configs are ignored.
          </p>
        </PlatformNote>
      </div>

      <div className="about-section">
        <DocsH2 id="troubleshooting">Troubleshooting</DocsH2>
        <div className="install-troubleshooting">
          <details className="install-trouble">
            <summary>A security popup appears when I launch</summary>
            <div className="install-trouble__body">
              <ul>
                <li>
                  <strong>Windows:</strong> SmartScreen may flag the unsigned
                  installer or game on first run. Click "More info" → "Run
                  anyway."
                </li>
                <li>
                  <strong>macOS:</strong> click "Open" in the first-launch
                  dialog.
                </li>
              </ul>
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
              <ul>
                <li>
                  <strong>Are you receiving voice at all?</strong> When someone
                  speaks, a speaker icon shows up over their head and in the
                  talker stack at the upper right of the HUD. If those never
                  appear, you're not getting voice from the server.
                </li>
                <li>
                  <strong>Voice volume.</strong> Voice has its own volume (
                  <code>cl_voipVolume</code>), separate from main and music.
                </li>
                <li>
                  <strong>Client setting.</strong> <code>cl_voip 1</code>{" "}
                  (default).
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
                  <strong>Speaker with an X next to it.</strong> You're muted;
                  press <strong>VoIP PTT / Toggle</strong> to unmute.
                </li>
                <li>
                  <strong>Speaker with no sound waves.</strong> In push-to-talk,
                  hold your <strong>VoIP PTT / Toggle</strong> binding; with
                  voice activity, speak above <code>cl_voipVADThreshold</code>.
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
