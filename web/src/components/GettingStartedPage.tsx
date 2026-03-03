import { Header } from "./Header";
import { useGitHubReleases } from "../hooks/useGitHubReleases";

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

interface FeatureItem {
  name: string;
  cvar?: string;
  desc: string;
  video?: string;
}

const FEATURES: FeatureItem[] = [
  {
    name: "VR Tracking",
    desc: "1:1 head and weapon hand tracking in VR — your player's head and weapon move exactly as you do.",
    video: "vr_tracking",
  },
  {
    name: "Damage Plums",
    cvar: "cg_damagePlums 1",
    desc: "Floating damage numbers appear on each hit, showing exactly how much damage you dealt.",
    video: "cg_damagePlums",
  },
  {
    name: "Blood Particles",
    cvar: "cg_bloodParticles 1",
    desc: "Particle-based blood effects with wall and floor splats, replacing the default sprite blood.",
    video: "cg_bloodParticles",
  },
  {
    name: "Damage Effect",
    cvar: "cg_damageEffect 1",
    desc: "Directional red vignette when taking damage, replacing the default blood splatter overlay.",
    video: "cg_damageEffect",
  },
  {
    name: "Orbit Camera",
    cvar: "cg_followMode 1 / cg_smoothFollow 1",
    desc: "Third-person orbit camera for spectating, with smooth transitions between players.",
    video: "orbit_camera",
  },
  {
    name: "TV Demo Scrubbing",
    cvar: "+tv_scrub",
    desc: "Scrub forward and backward through recorded TV demos.",
    video: "tvd_scrub",
  },
  {
    name: "TV Demo Pause",
    cvar: "demopause",
    desc: "Pause and resume TV demo playback.",
    video: "tvd_pause",
  },
];

export function GettingStartedPage() {
  const { releases } = useGitHubReleases();

  return (
    <div className="about-page">
      <Header title="Getting Started" className="about-header" />

      <div className="about-content">
        <div className="about-section">
          <h2>Install or Update Trinity</h2>
          <p>
            Downloading these builds is the only way to enjoy all Trinity
            features. All engine downloads include the Trinity mod that was
            current at time of release. Stop by the{" "}
            <a href="https://discord.gg/tuDB2YNc7h">Team Beef Discord</a> if you
            have questions or want to connect.
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
          <h2>Game Files</h2>
          <p>
            Trinity includes all of the free content needed to play online, but
            if you own Quake 3 Arena or Quake 3: Team Arena, you can copy your
            retail <code>.pk3</code> files into the install directory for the
            full game experience, including the single-player campaign and many
            additional maps. Most public servers are dedicated to full game
            clients, so having these files also means you can play on the
            majority of them. The{" "}
            <a href="https://store.steampowered.com/app/2200/Quake_III_Arena/">
              Steam version of Quake 3 Arena
            </a>{" "}
            includes both Q3A and Team Arena.
          </p>
          <ul>
            <li>
              Copy your Quake 3 Arena <code>.pk3</code> files to the{" "}
              <code>baseq3</code> folder
            </li>
            <li>
              Copy your Team Arena <code>.pk3</code> files to the{" "}
              <code>missionpack</code> folder
            </li>
          </ul>
          <p>
            These files are typically named <code>pak0.pk3</code> through{" "}
            <code>pak8.pk3</code> for Quake 3 Arena and{" "}
            <code>pak0.pk3</code> through <code>pak3.pk3</code> for Team Arena.
            You can find them in your existing Quake 3 install directory, or
            from a Steam or GOG copy of the game.
          </p>
        </div>

        <div className="about-section">
          <h2>Your Player Identity (GUID)</h2>
          <p>
            Your GUID is how Trinity identifies you. It's generated from a{" "}
            <code>qkey</code> file unique to your installation. If you play on
            multiple devices or engines, each one will generate its own{" "}
            <code>qkey</code>, giving you a different identity on each.
          </p>
          <p>To keep a consistent identity across installations:</p>
          <ul>
            <li>
              Copy your <code>qkey</code> file between installations so they
              share the same identity
            </li>
            <li>
              Set <code>cl_guidServerUniq 0</code> in your config for a
              consistent GUID across all servers
            </li>
          </ul>
          <p>
            Trinity Engine defaults <code>cl_guidServerUniq</code> to{" "}
            <code>0</code>, so no action is needed there. Quake 3 VR and
            Quake3Quest default to <code>1</code>, which produces a different
            GUID per server — setting it to <code>0</code> is recommended.
          </p>
          <p>
            If you end up with multiple GUIDs, you can link them together on
            your <a href="/account">Account</a> page.
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

          <h3 className="about-features-heading">Trinity Features</h3>
          <div className="about-features">
            {FEATURES.map((f) => (
              <details key={f.name} className="about-feature">
                <summary>
                  <span className="about-feature-name">{f.name}</span>
                  {f.cvar && (
                    <code className="about-feature-cvar">{f.cvar}</code>
                  )}
                </summary>
                <div className="about-feature-content">
                  <p>{f.desc}</p>
                  {f.video && (
                    <video autoPlay loop muted playsInline>
                      <source
                        src={`/assets/videos/${f.video}.webm`}
                        type="video/webm"
                      />
                      <source
                        src={`/assets/videos/${f.video}.mp4`}
                        type="video/mp4"
                      />
                    </video>
                  )}
                </div>
              </details>
            ))}
          </div>
        </div>

        <div className="about-section">
          <h2>Server Administration</h2>
          <p>
            These cvars are available for server operators running Trinity.
          </p>
          <ul>
            <li>
              <code>g_overtimelimit</code> (default: <code>0</code>) — overtime
              duration in minutes for tied matches (0 = sudden death)
            </li>
            <li>
              <code>g_teamDMSpawnThreshold</code> (default: <code>8</code>) —
              use team/CTF spawn points on maps with fewer FFA spawns than this
              value (avoids telefrag-fests in Team DM)
            </li>
            <li>
              <code>sv_tvAuto</code> (default: <code>0</code>) — automatically
              start TV recording on map load
            </li>
            <li>
              <code>sv_tvAutoMinPlayers</code> (default: <code>0</code>) —
              minimum concurrent non-spectator human players to keep an
              auto-recording (0 = always keep)
            </li>
            <li>
              <code>sv_tvAutoMinPlayersSecs</code> (default: <code>0</code>) —
              seconds the player threshold must be continuously met (0 =
              instantaneous)
            </li>
            <li>
              <code>sv_tvpath</code> (default: <code>demos</code>) — directory
              for TV recordings
            </li>
            <li>
              <code>sv_tvDownload</code> (default: <code>0</code>) — notify
              clients to download TV recordings via HTTP at end of match
              (requires <code>sv_dlURL</code>)
            </li>
            <li>
              <code>sv_dlURL</code> (default: empty) — base URL for HTTP
              downloads (pk3 and tvd)
            </li>
          </ul>
        </div>
      </div>
    </div>
  );
}
