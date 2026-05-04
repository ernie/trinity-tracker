package setup

import (
	"fmt"
	"os"
	"path/filepath"
)

const modRepo = "ernie/trinity"

// Mod paks and their install subdirs. The naming is historical and
// non-obvious: pak3t.pk3 is the missionpack mod (config-missionpack.mk
// sets PK3 = pak3t.pk3) and pak8t.pk3 is the baseq3 mod
// (config-baseq3.mk sets PK3 = pak8t.pk3). Don't try to "fix" the
// pairing — q3's vfs uses these as load-order slots, not mod identifiers.
var modPaks = []struct {
	Asset, Subdir string
}{
	{"pak3t.pk3", "missionpack"},
	{"pak8t.pk3", "baseq3"},
}

// InstallMod overlays the latest mod pk3s on top of whatever the
// engine bundled. Each pak is downloaded to a staging file owned by
// uid/gid in the same directory it'll land in (so the rename-into-
// place is atomic on the same filesystem), verified against the
// release's sha256sums.txt, and then renamed onto the live pk3.
//
// Caller is responsible for stopping any running trinity.ded
// instances first — q3 holds pk3s open via mmap, and a rename swaps
// the inode while the running process keeps the old one (so it won't
// crash, but it won't see the new mod until restart either).
//
// Returns the resolved tag (whatever "latest" mapped to, or the
// explicit tag passed in).
func InstallMod(plan *Plan, tag, quake3Dir string, uid, gid int) (string, error) {
	resolvedTag := tag
	if resolvedTag == "" {
		t, err := ResolveLatestTag(modRepo)
		if err != nil {
			return "", fmt.Errorf("resolving latest mod tag: %w", err)
		}
		resolvedTag = t
	}

	if plan.DryRun {
		for _, m := range modPaks {
			plan.Say("would download %s/%s and install to %s/%s/%s",
				modRepo, m.Asset, quake3Dir, m.Subdir, m.Asset)
		}
		return resolvedTag, nil
	}

	for _, m := range modPaks {
		destDir := filepath.Join(quake3Dir, m.Subdir)
		if err := plan.MkdirChown(destDir, 0755, uid, gid); err != nil {
			return "", err
		}
		// Stage adjacent to the destination so rename(2) is atomic.
		// Download runs as the service user via curl so the file lands
		// service-owned; we lose DownloadReleaseAsset's gh fallback,
		// which doesn't matter for the mod (public releases, no auth).
		stagePath := filepath.Join(destDir, "."+m.Asset+".new")
		url := ReleaseAssetURL(modRepo, resolvedTag, m.Asset)
		if err := plan.DownloadAs(uid, gid, url, stagePath); err != nil {
			_ = os.Remove(stagePath)
			return "", err
		}
		if err := VerifyReleaseChecksum(modRepo, resolvedTag, m.Asset, stagePath); err != nil {
			_ = os.Remove(stagePath)
			return "", err
		}
		if err := os.Chmod(stagePath, 0644); err != nil {
			_ = os.Remove(stagePath)
			return "", fmt.Errorf("chmod %s: %w", stagePath, err)
		}
		dest := filepath.Join(destDir, m.Asset)
		if err := os.Rename(stagePath, dest); err != nil {
			_ = os.Remove(stagePath)
			return "", fmt.Errorf("rename %s → %s: %w", stagePath, dest, err)
		}
		plan.Say("Installed %s → %s", m.Asset, dest)
	}
	return resolvedTag, nil
}
