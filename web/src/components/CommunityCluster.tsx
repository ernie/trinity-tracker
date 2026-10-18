import { DiscordButton } from "./DiscordButton";
import { GitHubButton } from "./GitHubButton";
import { DownloadIcon } from "./DownloadIcon";
import { InstallerDownloadLink } from "./InstallerDownloadLink";
import { useInstallerDownload } from "../hooks/useInstallerDownload";

export function CommunityCluster() {
  const installer = useInstallerDownload();
  return (
    <div className="community-cluster" aria-label="Community links">
      <InstallerDownloadLink
        download={installer}
        className="download-toggle"
        title={
          installer.direct
            ? `Download the Trinity Installer for ${installer.platform}`
            : "Install Trinity"
        }
      >
        <DownloadIcon size={16} />
      </InstallerDownloadLink>
      <DiscordButton />
      <GitHubButton />
    </div>
  );
}
