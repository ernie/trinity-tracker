import { useGitHubReleases } from "../../hooks/useGitHubReleases";

const DOWNLOAD_DESCRIPTIONS: Record<string, string> = {
  trinity: "Custom Quake 3 mod with Trinity features",
  "trinity-engine": "Flatscreen engine based on Quake3e",
  q3vr: "VR engine for PC VR headsets",
  ioq3quest: "VR engine for Meta Quest (2, 3, or 3S)",
};

const CONFIG_DOWNLOADS = [
  {
    name: "Trinity Engine (Flatscreen)",
    href: "/configs/trinity-autoexec.cfg",
    desc: "Quake3e-based engine for desktop",
  },
  {
    name: "Quake 3 VR",
    href: "/configs/q3vr-autoexec.cfg",
    desc: "PCVR engine — includes controller bindings",
  },
  {
    name: "Quake3Quest",
    href: "/configs/ioq3quest-autoexec.cfg",
    desc: "Quest 2, 3, and 3S — includes controller bindings",
  },
];

export function DocsGettingStarted() {
  const { releases } = useGitHubReleases();

  return (
    <>
      <div className="about-section">
        <h2>Install Trinity</h2>
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
          the <a href="https://discord.gg/tuDB2YNc7h">Team Beef Discord</a>{" "}
          if you have questions or want to connect.
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
        <h2>Your Account</h2>
        <p>
          Type <code>!claim</code> in the in-game chat to create an account.
          You'll receive a code to set up your username and password. Once
          you have an account, log in via the main menu's Account option —
          your identity on that client will be linked automatically. Log in
          on each engine or device you play from to link them all.
        </p>
        <p>
          Without an account, each installation has its own identity based
          on a <code>qkey</code> file. To keep your stats connected across
          devices, you'll need to copy your <code>qkey</code> between
          installations, set <code>cl_guidServerUniq 0</code> so your
          identity stays the same across servers, and use{" "}
          <code>!link</code> to merge any separate identities.
        </p>
      </div>

      <div className="about-section">
        <h2>Automatic Updates</h2>
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

      <div className="about-section">
        <h2>Configuration</h2>
        <p>
          Trinity adds gameplay features beyond base Quake 3. Create an{" "}
          <code>autoexec.cfg</code> in your <code>baseq3</code> folder to
          enable them, or download a suggested starting point for your engine:
        </p>
        <div className="about-downloads">
          {CONFIG_DOWNLOADS.map((c) => (
            <a
              key={c.href}
              href={c.href}
              download="autoexec.cfg"
              className="about-download-item"
            >
              <div className="about-download-info">
                <span className="about-download-name">{c.name}</span>
                <span className="about-download-desc">{c.desc}</span>
              </div>
              <span className="about-download-version">autoexec.cfg</span>
            </a>
          ))}
        </div>
      </div>
    </>
  );
}
