import { useState, useEffect } from "react";

export interface ReleaseInfo {
  repo: string;
  displayName: string;
  version: string | null;
  url: string;
  assetUrl?: string;
  bundled: boolean;
}

// Shape of the bits of GitHub's /releases/latest response we consume.
interface GitHubAsset {
  name: string;
  browser_download_url: string;
}

interface RepoConfig {
  repo: string;
  displayName: string;
  bundled: boolean;
  // Optional asset selector — when the API call succeeds, the matching
  // asset's browser_download_url is exposed as ReleaseInfo.assetUrl.
  // Use for repos whose canonical download is version-stamped (e.g.,
  // an APK named with the release tag) where /releases/latest/download
  // can't be used with a constant filename. Repos with stable asset
  // names should skip the matcher and hardcode the redirect URL at the
  // call site instead.
  assetMatcher?: (asset: GitHubAsset) => boolean;
}

const REPOS: RepoConfig[] = [
  { repo: "trinity", displayName: "Trinity Mod", bundled: false },
  { repo: "trinity-engine", displayName: "Trinity Engine", bundled: true },
  { repo: "trinity-vr", displayName: "Trinity VR", bundled: true },
  {
    repo: "trinity-quest",
    displayName: "Trinity Quest",
    bundled: true,
    // APK is published as trinity-quest-<ver>.apk (version-stamped, so
    // /releases/latest/download can't use a constant filename here).
    assetMatcher: (a) => /^trinity-quest-.*\.apk$/.test(a.name),
  },
];

const CACHE_KEY = "github-releases";
const CACHE_TTL = 60 * 60 * 1000; // 1 hour

interface CacheEntry {
  ts: number;
  releases: ReleaseInfo[];
}

function getCached(): ReleaseInfo[] | null {
  try {
    const raw = sessionStorage.getItem(CACHE_KEY);
    if (!raw) return null;
    const entry: CacheEntry = JSON.parse(raw);
    if (Date.now() - entry.ts > CACHE_TTL) return null;
    return entry.releases;
  } catch {
    return null;
  }
}

function setCache(releases: ReleaseInfo[]) {
  try {
    const entry: CacheEntry = { ts: Date.now(), releases };
    sessionStorage.setItem(CACHE_KEY, JSON.stringify(entry));
  } catch {
    // sessionStorage full or unavailable
  }
}

export function useGitHubReleases() {
  const [releases, setReleases] = useState<ReleaseInfo[]>(
    () =>
      getCached() ??
      REPOS.map((r) => ({
        repo: r.repo,
        displayName: r.displayName,
        version: null,
        url: `https://github.com/ernie/${r.repo}/releases`,
        bundled: r.bundled,
      })),
  );
  const [loading, setLoading] = useState(() => getCached() === null);

  useEffect(() => {
    // Cache hit was applied during state init — nothing to fetch.
    if (getCached()) return;

    const promises = REPOS.map((r) =>
      fetch(`https://api.github.com/repos/ernie/${r.repo}/releases/latest`)
        .then((res) => {
          if (!res.ok) throw new Error(`${res.status}`);
          return res.json();
        })
        .then((data) => {
          const assets: GitHubAsset[] = Array.isArray(data.assets)
            ? data.assets
            : [];
          const matched = r.assetMatcher
            ? assets.find(r.assetMatcher)
            : undefined;
          return {
            repo: r.repo,
            displayName: r.displayName,
            version: data.tag_name as string,
            url: `https://github.com/ernie/${r.repo}/releases/latest`,
            assetUrl: matched?.browser_download_url,
            bundled: r.bundled,
          };
        })
        .catch(() => ({
          repo: r.repo,
          displayName: r.displayName,
          version: null,
          url: `https://github.com/ernie/${r.repo}/releases`,
          bundled: r.bundled,
        })),
    );

    Promise.all(promises).then((results) => {
      setReleases(results);
      setCache(results);
      setLoading(false);
    });
  }, []);

  return { releases, loading };
}
