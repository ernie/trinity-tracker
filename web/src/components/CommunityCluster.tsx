import { DiscordButton } from "./DiscordButton";
import { GitHubButton } from "./GitHubButton";
import { DownloadIcon } from "./DownloadIcon";
import { InstallerDownloadLink } from "./InstallerDownloadLink";
import { useInstallerDownload } from "../hooks/useInstallerDownload";

// Groups external/social affordances together, separated from the
// primary nav. Order: Install (downloads the Trinity Installer for the
// visitor's OS, or opens the install guide where it doesn't run),
// Discord (community), GitHub (source). All three render as same-size
// icon buttons so the cluster reads as a single unit.
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
