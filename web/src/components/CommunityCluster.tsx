import { Link } from "react-router-dom";
import { DiscordButton } from "./DiscordButton";
import { GitHubButton } from "./GitHubButton";
import { DownloadIcon } from "./DownloadIcon";

// Groups external/social affordances together, separated from the
// primary nav. Order: Install (gateway to the app — links to the
// install docs which lay out the full prerequisite flow rather than
// promising a one-click download), Discord (community), GitHub
// (source). All three render as same-size icon buttons so the
// cluster reads as a single unit.
export function CommunityCluster() {
  return (
    <div className="community-cluster" aria-label="Community links">
      <Link
        to="/docs/install"
        className="download-toggle"
        title="Install Trinity"
        aria-label="Install Trinity"
      >
        <DownloadIcon size={16} />
      </Link>
      <DiscordButton />
      <GitHubButton />
    </div>
  );
}
