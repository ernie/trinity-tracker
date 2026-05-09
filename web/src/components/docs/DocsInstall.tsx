import { useGitHubReleases } from '../../hooks/useGitHubReleases'
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

      {/* Subsequent tasks (Steps 2-5) will append their about-section
          blocks here. */}
    </>
  )
}
