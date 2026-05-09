import { Link } from "react-router-dom";
import { useGitHubReleases } from "../../hooks/useGitHubReleases";
import { DISCORD_INVITE_URL } from "../../constants/discord";
import { DocsH2 } from "./DocsH2";

const DOWNLOAD_DESCRIPTIONS: Record<string, string> = {
  trinity: "Custom Quake 3 mod with Trinity features",
  "trinity-engine": "Flatscreen engine based on Quake3e",
  q3vr: "VR engine for PC VR headsets",
  ioq3quest: "VR engine for Meta Quest (2, 3, or 3S)",
};

// New /docs/install page — extracted from DocsGettingStarted as part
// of the Phase 2 IA split. Content is unchanged in this phase; tone
// + platform-aware variants land in a later phase.
export function DocsInstall() {
  const { releases } = useGitHubReleases();

  return (
    <>
      <div className="about-section">
        <DocsH2 id="install-trinity">Install Trinity</DocsH2>
        <p>
          Downloading these builds is the only way to enjoy all Trinity
          features. All engine downloads include the Trinity mod that was
          current at time of release. If you own{" "}
          <a href="https://store.steampowered.com/app/2200/Quake_III_Arena/">
            Quake 3 Arena on Steam
          </a>
          , copy your <code>baseq3</code> and <code>missionpack</code>{" "}
          <code>.pk3</code> files into the matching folders in your Trinity
          install — most public servers require the full game assets. Stop by
          the <a href={DISCORD_INVITE_URL}>Trinity Discord</a>{" "}
          if you have questions or want to connect.
        </p>
        <p>
          The free Quake 3 demo (evaluation version) is not supported —
          Trinity targets retail Quake 3 only. Running demo servers or
          clients against Trinity is entirely on you.
        </p>
        <p>
          Need the 1.32 point-release patch data?{" "}
          <Link to="/quake3-eula">Read the id Software EULA and download here</Link>.
        </p>
        <div className="about-downloads">
          {releases.map((r) => (
            <div key={r.repo}>
              <a
                href={r.url}
                target="_blank"
                rel="noopener noreferrer"
                className="about-download-item"
              >
                <div className="about-download-info">
                  <span className="about-download-name">
                    {r.displayName}
                    {r.bundled && (
                      <span className="about-download-bundled">
                        <img
                          src="/assets/icon-128.png"
                          alt=""
                          className="about-download-bundled-icon"
                        />
                        Includes Trinity mod
                      </span>
                    )}
                  </span>
                  <span className="about-download-desc">
                    {DOWNLOAD_DESCRIPTIONS[r.repo]}
                  </span>
                </div>
                {r.version && (
                  <span className="about-download-version">{r.version}</span>
                )}
              </a>
              {r.repo === "trinity" && (
                <div className="about-download-install-note">
                  Copy <code>pak8t.pk3</code> to your <code>baseq3</code>{" "}
                  folder and <code>pak3t.pk3</code> to your{" "}
                  <code>missionpack</code> folder.
                </div>
              )}
            </div>
          ))}
        </div>
      </div>

      <div className="about-section">
        <DocsH2 id="automatic-updates">Automatic Updates</DocsH2>
        <p>
          Trinity checks for new releases on startup. When an update
          is available, an indicator appears on the main menu. From there you
          can download and apply the update without leaving the game.
        </p>
        <p>
          You can also manage updates from the console:
        </p>
        <ul>
          <li>
            <code>update</code> — check for updates
          </li>
          <li>
            <code>updatedownload</code> — download an available update
          </li>
          <li>
            <code>updatecancel</code> — cancel an in-progress download
          </li>
          <li>
            <code>updaterestart</code> — apply a downloaded update and restart
          </li>
        </ul>
        <p>
          To disable the automatic check, set <code>update_check 0</code> in
          your config.
        </p>
      </div>
    </>
  );
}
