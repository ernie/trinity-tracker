import { useGitHubReleases } from "./useGitHubReleases";

export type DetectedOS = "windows" | "macos" | "linux";

// Coarse desktop-OS detection from the UA string. Returns null on
// mobile or anything we don't recognize. Mobile checks come first
// because Android contains "Linux" in its UA and iOS contains "Mac OS X".
export function detectOS(): DetectedOS | null {
  if (typeof navigator === "undefined") return null;
  const ua = navigator.userAgent;
  if (/Android/i.test(ua)) return null;
  if (/iPhone|iPad|iPod/i.test(ua)) return null;
  if (/Windows/i.test(ua)) return "windows";
  if (/Mac OS X|Macintosh/i.test(ua)) return "macos";
  if (/Linux|X11/i.test(ua)) return "linux";
  return null;
}

const INSTALLER_ASSETS: Record<DetectedOS, { asset: string; label: string }> = {
  windows: {
    asset: "trinity-installer-windows-x86_64.exe",
    label: "Windows",
  },
  macos: { asset: "trinity-installer-macos.zip", label: "macOS" },
  linux: {
    asset: "trinity-installer-linux-x86_64.tar.gz",
    label: "Linux",
  },
};

export interface InstallerDownload {
  // Direct download of the installer, or the install guide when the
  // visitor isn't on a desktop OS the installer runs on.
  href: string;
  direct: boolean;
  platform: string | null;
  version: string | null;
}

export function useInstallerDownload(): InstallerDownload {
  const { releases } = useGitHubReleases();
  const version =
    releases.find((r) => r.repo === "trinity-installer")?.version ?? null;
  const os = detectOS();
  if (!os) {
    return { href: "/docs/install", direct: false, platform: null, version };
  }
  return {
    href: `https://github.com/ernie/trinity-installer/releases/latest/download/${INSTALLER_ASSETS[os].asset}`,
    direct: true,
    platform: INSTALLER_ASSETS[os].label,
    version,
  };
}
