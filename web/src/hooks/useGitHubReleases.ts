import { useState, useEffect } from "react";

export interface ReleaseInfo {
  repo: string;
  displayName: string;
  version: string | null;
  url: string;
  bundled: boolean;
}

interface RepoConfig {
  repo: string;
  displayName: string;
  bundled: boolean;
}

const REPOS: RepoConfig[] = [
  { repo: "trinity", displayName: "Trinity Mod", bundled: false },
  { repo: "trinity-engine", displayName: "Trinity Engine", bundled: true },
  { repo: "trinity-vr", displayName: "Trinity VR", bundled: true },
  {
    repo: "trinity-standalone",
    displayName: "Trinity Standalone",
    bundled: true,
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

function placeholder(r: RepoConfig): ReleaseInfo {
  return {
    repo: r.repo,
    displayName: r.displayName,
    version: null,
    url: `https://github.com/ernie/${r.repo}/releases/latest`,
    bundled: r.bundled,
  };
}

export function useGitHubReleases() {
  const [releases, setReleases] = useState<ReleaseInfo[]>(
    () => getCached() ?? REPOS.map(placeholder),
  );
  const [loading, setLoading] = useState(() => getCached() === null);

  useEffect(() => {
    // Cache hit was applied during state init — nothing to fetch.
    if (getCached()) return;

    const promises: Promise<{ ok: boolean; release: ReleaseInfo }>[] =
      REPOS.map((r) =>
        fetch(`https://api.github.com/repos/ernie/${r.repo}/releases/latest`)
          .then((res) => {
            if (!res.ok) throw new Error(`${res.status}`);
            return res.json();
          })
          .then((data) => ({
            ok: true,
            release: {
              repo: r.repo,
              displayName: r.displayName,
              version: data.tag_name as string,
              url: `https://github.com/ernie/${r.repo}/releases/latest`,
              bundled: r.bundled,
            },
          }))
          .catch(() => ({ ok: false, release: placeholder(r) })),
      );

    Promise.all(promises).then((results) => {
      setReleases(results.map((r) => r.release));
      // Caching a failed lookup would pin this tab to it for the full TTL.
      if (results.every((r) => r.ok)) {
        setCache(results.map((r) => r.release));
      }
      setLoading(false);
    });
  }, []);

  return { releases, loading };
}
