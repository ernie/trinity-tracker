// Bespoke landing hero header — brand · sparse nav · download + live pill.
// Replaces the standard <Header> over the wallpaper.
import { Link } from "react-router-dom";
import { useLiveData } from "../../contexts/LiveDataContext";
import { useInstallerDownload } from "../../hooks/useInstallerDownload";
import { DownloadIcon } from "../DownloadIcon";
import { InstallerDownloadLink } from "../InstallerDownloadLink";
import { derivePillState } from "../pillState";

export function HeroHeader() {
  const { activeHumanPlayersCount, connectionStatus } = useLiveData();
  const installer = useInstallerDownload();
  // Shared with StatusPill: live (humans fragging), quiet (hub up or still
  // connecting, arena empty), offline (the feed is genuinely unreachable).
  // Class name `quiet` is kept as an internal CSS hook even though the
  // visible label is "STANDING BY".
  const { stateClass, label } = derivePillState(
    connectionStatus,
    activeHumanPlayersCount,
  );

  return (
    <header className="hero__header">
      <Link to="/" className="hero__brand">
        <img
          className="hero__brand-logo"
          src="/assets/icon-1104.png"
          alt=""
          aria-hidden
        />
        <span className="hero__brand-title">
          Trinity<span className="hero__brand-wordmark">tracker</span>
        </span>
      </Link>

      <nav className="hero__nav" aria-label="Primary">
        <Link to="/servers">Servers</Link>
        <Link to="/matches">Matches</Link>
        <Link to="/players">Players</Link>
        <Link to="/leaderboard">Leaderboard</Link>
        <Link to="/docs">Docs</Link>
      </nav>

      <div className="hero__actions">
        <InstallerDownloadLink
          download={installer}
          className="hero__download"
          title={
            installer.direct
              ? `Download the Trinity Installer${installer.version ? ` ${installer.version}` : ""} for ${installer.platform}`
              : undefined
          }
        >
          <DownloadIcon size={14} />
          Install
        </InstallerDownloadLink>
        <span className={`hero__pill ${stateClass}`} aria-live="polite">
          <span className="dot" aria-hidden />
          {label}
        </span>
      </div>
    </header>
  );
}
