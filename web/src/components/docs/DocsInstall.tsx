import { Link } from "react-router-dom";
import { useGitHubReleases } from "../../hooks/useGitHubReleases";
import { useInstallerDownload } from "../../hooks/useInstallerDownload";
import { DISCORD_INVITE_URL } from "../../constants/discord";
import { DocsH2 } from "./DocsH2";
import { PlatformNote } from "./PlatformNote";

export function DocsInstall() {
  const { releases } = useGitHubReleases();
  const installer = useInstallerDownload();
  const installerRelease = releases.find((r) => r.repo === "trinity-installer");

  return (
    <>
      <div className="about-section">
        <DocsH2 id="get-quake3">Step 1: Get Quake III Arena</DocsH2>
        <p>
          Trinity needs a retail Quake III Arena and, optionally, Team Arena; it
          uses their <code>pak0.pk3</code> files.
        </p>
        <p>
          Buy{" "}
          <a
            href="https://store.steampowered.com/app/2200/Quake_III_Arena/"
            target="_blank"
            rel="noopener noreferrer"
          >
            Quake III Arena on Steam
          </a>{" "}
          (or use a retail CD install) and install it on a PC or Mac. Steam's
          release ships both the base game and Team Arena. The Trinity Installer
          copies the game files from there, wherever you're installing Trinity.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="install-trinity">Step 2: Run the Trinity Installer</DocsH2>
        <p>
          The <strong>Trinity Installer</strong> downloads the latest Trinity
          for the device you pick, copies your Quake III game files (fetching
          the 1.32 patch files if your install lacks them, after you accept id
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
          Run it on the computer with Quake III installed and choose where
          Trinity goes:
        </p>
        <ul>
          <li>
            <strong>This PC or Mac:</strong> nothing else to prepare. On Windows
            it installs to <code>C:\Games\Trinity</code> by default. On Windows
            and Linux you pick whether shortcuts start in VR or flatscreen, and
            "Add to Steam" adds Trinity to your Steam library, where the VR
            shortcut also shows up in SteamVR. With Add to Steam chosen, the VR
            shortcut starts Trinity through Steam, so SteamVR goes straight to
            the game.
          </li>
          <li>
            <strong>Steam Frame:</strong> turn on Developer Mode on the headset,
            then pair it from{" "}
            <strong>Settings → Developer → Pair new host</strong> when the
            installer asks.
          </li>
          <li>
            <strong>Meta Quest or PICO:</strong> turn on Developer Mode and USB
            debugging, connect the headset over USB, and accept the debugging
            prompt in the headset.
          </li>
        </ul>
        <PlatformNote platform="vr">
          <p>
            On a PC, one Trinity install plays both flatscreen and VR, through
            SteamVR or any OpenXR runtime; pick one at{" "}
            <strong>Setup → Display → Display Mode</strong>. On a Steam Frame,
            Quest, or PICO, Trinity runs on the headset itself and plays VR
            only, with the Vulkan renderer and no Display Mode setting. You need
            the computer only to install it.
          </p>
        </PlatformNote>
      </div>

      <div className="about-section">
        <DocsH2 id="automatic-updates">Automatic updates</DocsH2>
        <p>
          Trinity checks for new releases on startup. When an update is
          available, an indicator appears on the main menu, where you can
          download and apply it. On a PC, Mac, or Steam Frame, Trinity then
          relaunches itself.
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
                Only servers running the Trinity collector report stats. Trinity
                can play on standard Quake III servers, but those don&rsquo;t
                report matches anywhere. Servers that report to this site are
                labeled on the <Link to="/servers">Servers</Link> page.
              </p>
              <p>
                The hub tracks you with or without a Trinity account. An account
                helps when your stats end up split across installs.
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
                separate player to the hub. If you&rsquo;ve installed on more
                than one machine, or reinstalled, your stats can end up split
                across two or three &ldquo;yous,&rdquo; even under the same
                in-game name.
              </p>
              <p>
                Sign in to a Trinity account on every device you play on, and
                the hub merges your installs automatically. The{" "}
                <Link to="/docs/account">Account</Link> page covers signing in.
              </p>
            </div>
          </details>

          <details className="install-trouble">
            <summary>I can't hear other players on voice chat</summary>
            <div className="install-trouble__body">
              <ul>
                <li>
                  When someone speaks, a speaker icon shows up over their head
                  and in the talker stack at the upper right of the HUD. If
                  those never appear, you&rsquo;re not getting voice from the
                  server.
                </li>
                <li>
                  Check <strong>Setup → System → Sound</strong>: <em>VOIP</em>{" "}
                  must be on (the default), and voice has its own{" "}
                  <em>VOIP Volume</em>, separate from the main and music
                  volumes.
                </li>
                <li>
                  If the indicators never appear no matter who&rsquo;s playing,
                  the server probably has voice chat turned off. Try a different
                  server, or ask in the{" "}
                  <a href={DISCORD_INVITE_URL}>Trinity Discord</a>.
                </li>
              </ul>
            </div>
          </details>

          <details className="install-trouble">
            <summary>People can't hear me on voice chat</summary>
            <div className="install-trouble__body">
              <p>
                The speaker icon on your own player portrait in the HUD shows
                whether you&rsquo;re transmitting:
              </p>
              <ul>
                <li>
                  <strong>Speaker with an X next to it.</strong> You're muted;
                  press <strong>VoIP PTT / Toggle</strong> to unmute.
                </li>
                <li>
                  <strong>Speaker with no sound waves.</strong> With{" "}
                  <em>Push to talk</em>, hold your{" "}
                  <strong>VoIP PTT / Toggle</strong> binding; with{" "}
                  <em>When speaking</em>, speak above the <em>Threshold</em> in{" "}
                  <strong>Setup → System → Sound</strong>.
                </li>
                <li>
                  <strong>Speaker with sound waves.</strong> You're
                  transmitting. If people still can&rsquo;t hear you, the
                  problem is the server&rsquo;s config or their end. Ask in the{" "}
                  <a href={DISCORD_INVITE_URL}>Trinity Discord</a> for help.
                </li>
              </ul>
            </div>
          </details>
        </div>
      </div>
    </>
  );
}
