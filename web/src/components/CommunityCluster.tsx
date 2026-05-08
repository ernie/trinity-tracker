import { DownloadButton } from './DownloadButton'
import { DiscordButton } from './DiscordButton'
import { GitHubButton } from './GitHubButton'

// Groups external/social affordances together, separated from the
// primary nav. Order: Download (gateway to the app), Discord (community),
// GitHub (source). All three render as same-size icon buttons so the
// cluster reads as a single unit.
export function CommunityCluster() {
  return (
    <div className="community-cluster" aria-label="Community links">
      <DownloadButton />
      <DiscordButton />
      <GitHubButton />
    </div>
  )
}
