import { Link } from 'react-router-dom'
import { useGitHubReleases } from '../../hooks/useGitHubReleases'
import { DISCORD_INVITE_URL } from '../../constants/discord'
import { DocsH2 } from './DocsH2'
import { PlatformTabs } from './PlatformTabs'
import { PlatformNote } from './PlatformNote'

// /docs/install — five-step install guide. Replaces the Phase 2
// verbatim lift from the old DocsGettingStarted with verified
// per-platform content. Each engine's binary is shown via
// PlatformTabs; small per-platform deviations use PlatformNote.
export function DocsInstall() {
  const { releases } = useGitHubReleases()

  // Map releases to platform — release.repo identifies which engine
  // a binary is for. trinity-engine = flatscreen, q3vr = pcvr,
  // ioq3quest = quest. The fourth ('trinity') is the mod itself,
  // bundled with each engine, not a standalone download here.
  const flatscreen = releases.find((r) => r.repo === 'trinity-engine')
  const pcvr = releases.find((r) => r.repo === 'q3vr')
  const quest = releases.find((r) => r.repo === 'ioq3quest')

  return (
    <>
      <div className="about-section">
        <DocsH2 id="install-trinity">Step 1 — Download Trinity</DocsH2>
        <p>
          Trinity ships as a custom Quake 3 engine plus the gameplay
          mod. Each engine targets a different way of playing — pick
          the one that matches your setup.
        </p>

        <PlatformTabs>
          <PlatformTabs.Panel platform="flatscreen">
            <p>
              <strong>Trinity Engine</strong> is a Quake3e-based
              build for desktop monitors with keyboard + mouse.
            </p>
            {flatscreen && (
              <a
                href={flatscreen.url}
                target="_blank"
                rel="noopener noreferrer"
                className="install-download-link"
              >
                Download Trinity Engine{flatscreen.version ? ` ${flatscreen.version}` : ''} →
              </a>
            )}
            <p>
              The download bundles the Trinity mod that was current
              at release time. The mod itself updates automatically
              after install (see Step 4).
            </p>
          </PlatformTabs.Panel>

          <PlatformTabs.Panel platform="pcvr">
            <p>
              <strong>Quake 3 VR</strong> is a PC-tethered VR build
              based on RippeR37's Q3VR. Runs on any PCVR headset
              (Index, Vive, Rift, tethered Quest, etc.) via SteamVR.
            </p>
            {pcvr && (
              <a
                href={pcvr.url}
                target="_blank"
                rel="noopener noreferrer"
                className="install-download-link"
              >
                Download Quake 3 VR{pcvr.version ? ` ${pcvr.version}` : ''} →
              </a>
            )}
          </PlatformTabs.Panel>

          <PlatformTabs.Panel platform="quest">
            <p>
              <strong>Quake3Quest</strong> runs natively on Meta
              Quest 2, 3, and 3S — no PC required. Built on Team
              Beef's Quake3Quest port.
            </p>
            {quest && (
              <a
                href={quest.url}
                target="_blank"
                rel="noopener noreferrer"
                className="install-download-link"
              >
                Download Quake3Quest{quest.version ? ` ${quest.version}` : ''} →
              </a>
            )}
            <p>
              Sideload the <code>.apk</code> using{' '}
              <a
                href="https://sidequestvr.com/"
                target="_blank"
                rel="noopener noreferrer"
              >
                SideQuest
              </a>
              {' '}— the same tool people use for the standard Team
              Beef ports.
            </p>
            <PlatformNote platform="quest">
              <p>
                <strong>Already have the Team Beef Quake3Quest
                installed?</strong> Uninstall it before installing
                Trinity's build — the two can't coexist on the same
                headset. While you're uninstalling, take the
                opportunity to clear any{' '}
                <code>autoexec.cfg</code> files from your{' '}
                <code>baseq3</code> and <code>missionpack</code>{' '}
                folders so Trinity starts with fresh settings.
              </p>
            </PlatformNote>
          </PlatformTabs.Panel>
        </PlatformTabs>
      </div>

      <div className="about-section">
        <DocsH2 id="copy-pak0">Step 2 — Copy <code>pak0.pk3</code> from your Quake 3 install</DocsH2>
        <p>
          Trinity is a mod, not a replacement game — you need a
          legitimate copy of Quake 3 Arena to play. The{' '}
          <code>pak0.pk3</code> file contains the original game
          assets and isn't redistributable, so you bring your own:
        </p>
        <ul>
          <li>
            Buy{' '}
            <a
              href="https://store.steampowered.com/app/2200/Quake_III_Arena/"
              target="_blank"
              rel="noopener noreferrer"
            >
              Quake 3 Arena on Steam
            </a>
            {' '}(or use a retail CD install).
          </li>
          <li>
            Locate <code>baseq3/pak0.pk3</code> in your Quake 3 install
            folder — typically{' '}
            <code>steamapps/common/Quake 3 Arena/baseq3/</code>.
          </li>
          <li>
            Copy it into Trinity's <code>baseq3</code> folder.
          </li>
        </ul>
        <PlatformNote platform="quest">
          <p>
            On Quest, Trinity's <code>baseq3</code> folder lives at{' '}
            <code>/sdcard/ioquake3Quest/baseq3/</code> on the headset's
            internal storage. The engine creates this directory on
            first launch. Use SideQuest's file browser, Android File
            Transfer, or <code>adb push</code> to copy{' '}
            <code>pak0.pk3</code> there.
          </p>
        </PlatformNote>
        <p>
          The 1.32 point-release patches in Step 3 fill in everything
          else (<code>pak1.pk3</code> through <code>pak8.pk3</code>{' '}
          plus missionpack patches) — without those, you'll have only
          the base <code>pak0.pk3</code> and most maps will fail to
          load.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="install-patches">Step 3 — Install the 1.32 point-release patches</DocsH2>
        <p>
          The Quake 3 1.32 point release adds <code>pak1.pk3</code>{' '}
          through <code>pak8.pk3</code> for <code>baseq3</code> plus
          the missionpack patches.{' '}
          <strong>This is required, not optional</strong> — most
          modern Quake 3 servers (Trinity included) need the full
          1.32 asset set to load maps and missionpack content.
        </p>
        <p>
          The patches ship under id Software's EULA, so we gate the
          download behind a quick read-and-accept page:{' '}
          <Link to="/quake3-eula">Read the EULA and download the 1.32 patches</Link>.
        </p>
        <p>
          Drop the resulting <code>pak1.pk3</code> through{' '}
          <code>pak8.pk3</code> files into your <code>baseq3</code>{' '}
          folder alongside <code>pak0.pk3</code>. The missionpack
          patches go into the <code>missionpack</code> folder.
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="automatic-updates">Step 4 — Automatic Updates</DocsH2>
        <p>
          Trinity checks for new releases on startup. When an update
          is available, an indicator appears on the main menu and you
          can download + apply it without leaving the game.
        </p>
        <p>
          You can also manage updates from the console. The first
          three commands work the same way on every engine; the
          command that applies a downloaded update differs per
          platform — pick your platform below for the specifics.
        </p>
        <ul>
          <li><code>update</code> — check for updates</li>
          <li><code>updatedownload</code> — download an available update</li>
          <li><code>updatecancel</code> — cancel an in-progress download</li>
        </ul>

        <PlatformTabs>
          <PlatformTabs.Panel platform="flatscreen">
            <p>
              <code>updaterestart</code> — apply a downloaded update
              and restart the game.
            </p>
          </PlatformTabs.Panel>
          <PlatformTabs.Panel platform="pcvr">
            <p>
              <code>updaterestart</code> — apply a downloaded update
              and restart the game.
            </p>
          </PlatformTabs.Panel>
          <PlatformTabs.Panel platform="quest">
            <p>
              <code>updateinstall</code> — hand the downloaded update
              to Android's package manager to install. This replaces
              the <code>updaterestart</code> command from the desktop
              engines (the Quest engine ships an <code>.apk</code>{' '}
              that Android installs through its own UI rather than
              restarting the game in place).
            </p>
          </PlatformTabs.Panel>
        </PlatformTabs>

        <p>
          To disable the automatic startup check, set{' '}
          <code>update_check 0</code> in your config (defaults to{' '}
          <code>1</code> on all engines).
        </p>
      </div>

      <div className="about-section">
        <DocsH2 id="troubleshooting">Step 5 — Troubleshooting</DocsH2>
        <p>
          The most common install issues, with what to check first.
        </p>
        <div className="install-troubleshooting">
          <details className="install-trouble">
            <summary>Maps fail to load — "<code>error: cannot find map …</code>"</summary>
            <div className="install-trouble__body">
              <p>
                You're missing the 1.32 point-release patches (Step 3).
                Without <code>pak1.pk3</code> through <code>pak8.pk3</code>,
                most maps don't load. Re-run the EULA flow at{' '}
                <Link to="/quake3-eula">/quake3-eula</Link> and copy the
                patches into your <code>baseq3</code> folder.
              </p>
            </div>
          </details>

          <details className="install-trouble">
            <summary>"<code>error: pak0.pk3 not found</code>" or "<code>missing assets</code>"</summary>
            <div className="install-trouble__body">
              <p>
                Trinity needs <code>pak0.pk3</code> from a legitimate
                Quake 3 install. Re-check Step 2 — the file should be
                sitting in Trinity's <code>baseq3</code> folder. If you
                copied it but it's still not found, confirm the case
                matches exactly (lowercase <code>pak0.pk3</code> on
                case-sensitive filesystems).
              </p>
            </div>
          </details>

          <PlatformNote platform={['flatscreen', 'pcvr']}>
            <details className="install-trouble">
              <summary>Trinity won't launch / Windows SmartScreen or macOS Gatekeeper blocks it</summary>
              <div className="install-trouble__body">
                <p>
                  Unsigned binaries trip SmartScreen (Windows) and
                  Gatekeeper (macOS) on first run. On Windows, click
                  "More info" → "Run anyway." On macOS, right-click
                  the app → Open, then confirm. Subsequent launches
                  won't prompt.
                </p>
              </div>
            </details>
          </PlatformNote>

          <PlatformNote platform="quest">
            <details className="install-trouble">
              <summary>Quest install fails — "another version is already installed"</summary>
              <div className="install-trouble__body">
                <p>
                  The Team Beef Quake3Quest is already on your
                  headset. Uninstall it through Quest's app library
                  before installing Trinity's build (the two can't
                  coexist — same package name). While you're at it,
                  clear any <code>autoexec.cfg</code> files from{' '}
                  <code>baseq3</code> and <code>missionpack</code> so
                  Trinity starts with fresh settings.
                </p>
              </div>
            </details>
          </PlatformNote>

          <details className="install-trouble">
            <summary>Stats aren't showing up on the hub</summary>
            <div className="install-trouble__body">
              <p>
                You're playing — but trinity.run isn't tracking your
                matches. Two common reasons:
              </p>
              <ul>
                <li>
                  The server you're on doesn't run the Trinity
                  collector. Trinity is backwards-compatible with
                  vanilla servers, but stats only flow from servers
                  that opted in. Check the server cards on{' '}
                  <Link to="/servers">/servers</Link> — they're
                  labeled.
                </li>
                <li>
                  Your client isn't linked to a Trinity account. See{' '}
                  <Link to="/docs/account">/docs/account</Link> —
                  claiming an account links your in-game identity to
                  leaderboard entries automatically.
                </li>
              </ul>
            </div>
          </details>

          <details className="install-trouble">
            <summary>I have multiple identities across machines / they aren't merging</summary>
            <div className="install-trouble__body">
              <p>
                Trinity identifies you by your client's{' '}
                <code>qkey</code> file, which is per-installation by
                default. Once you sign into a Trinity account, every
                device you log in from gets linked automatically. See{' '}
                <Link to="/docs/account#authentication">
                  Account → Authentication
                </Link>
                {' '}for the full link flow.
              </p>
            </div>
          </details>

          <details className="install-trouble">
            <summary>Voice chat isn't working / I can't hear anyone</summary>
            <div className="install-trouble__body">
              <p>
                Voice chat needs the server to have it enabled, and
                you need <code>cl_voip 1</code> set in your config.
                If your config is right and you still can't hear
                anyone, the server you're on probably doesn't have
                voice chat turned on — try a different server, or
                ask in the{' '}
                <a href={DISCORD_INVITE_URL}>Trinity Discord</a> for
                server-specific help.
              </p>
            </div>
          </details>
        </div>
      </div>
    </>
  )
}
