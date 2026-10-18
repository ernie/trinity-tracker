package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const modRepo = "ernie/trinity"

// Mod paks and their install subdirs. pak3t.pk3 is the missionpack mod
// and pak8t.pk3 the baseq3 mod (PK3 in the mod's config-missionpack.mk
// and config-baseq3.mk); the names are load-order slots in the game's
// file system.
var modPaks = []struct {
	Asset, Subdir string
}{
	{"pak3t.pk3", "missionpack"},
	{"pak8t.pk3", "baseq3"},
	{"zzz-trinity-announcer.pk3", "baseq3"},
}

// InstallMod overlays the latest mod pk3s on top of whatever the
// engine bundled. Each pak is staged beside its destination, verified
// against the release's sha256sums.txt, and renamed onto the live pk3.
//
// The caller stops running trinity.ded instances first: a running
// server keeps the old pk3 open and sees the new mod only after a restart.
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
		// Staged beside the destination so rename(2) is atomic, and
		// downloaded as the service user so the file is service-owned.
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

// CheckAnnouncerState reports whether the announcer pk3 at
// installedPath matches expectedChecksum. Returns StateCurrent when
// expectedChecksum is empty (release doesn't include the asset) or
// the hash matches, StateUnknown when the file is missing, and
// StateBehind when the hash differs.
func CheckAnnouncerState(installedPath, expectedChecksum string) (VersionState, string) {
	if expectedChecksum == "" {
		return StateCurrent, ""
	}
	actual, err := HashFile(installedPath)
	if err != nil {
		return StateUnknown, ""
	}
	if strings.EqualFold(expectedChecksum, actual) {
		return StateCurrent, actual
	}
	return StateBehind, actual
}
