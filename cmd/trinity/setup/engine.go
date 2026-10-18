package setup

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const engineRepo = "ernie/trinity-engine"

// engineBinary is the game server binary's name in every release zip,
// on every arch, once the linux-<arch>/ wrapper is stripped.
const engineBinary = "trinity.ded"

// engineAsset returns the release asset name for the given GOARCH (or
// the current host arch if goarch is empty). Returns an error for
// arches with no published build.
func engineAsset(goarch string) (string, error) {
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	switch goarch {
	case "amd64":
		return "trinity-linux-x86_64.zip", nil
	case "arm64":
		return "trinity-linux-arm64.zip", nil
	case "arm":
		return "trinity-linux-armv7.zip", nil
	case "386":
		return "trinity-linux-x86.zip", nil
	}
	return "", fmt.Errorf("unsupported arch %s; see https://github.com/%s/releases", goarch, engineRepo)
}

// InstallEngine downloads a trinity-engine release into installDir.
// Pass tag = "" for latest. Steps:
//
//  1. Resolve the tag (HEAD on releases/latest if tag is empty).
//  2. Fetch trinity-linux-<arch>.zip and verify against the
//     release's sha256sums.txt.
//  3. Extract into installDir, stripping the linux-<arch>/ wrapper.
//     If uid > 0, the extraction runs as that user through
//     `_helper unzip`, so files are service-owned from creation.
//  4. Replace baseq3/logs + missionpack/logs with symlinks to logDir
//     so per-server logs land where the collector tails them.
//  5. Replace baseq3/demos + missionpack/demos with symlinks to
//     staticDir/demos so recorded TVD demos land where nginx serves
//     them and the demo uploader picks them up.
//
// Returns the resolved tag (whatever "latest" resolved to, or the
// explicit tag passed in).
func InstallEngine(plan *Plan, tag, installDir, logDir, staticDir string, uid, gid int) (string, error) {
	asset, err := engineAsset("")
	if err != nil {
		return "", err
	}
	if err := plan.MkdirAll(installDir, 0755); err != nil {
		return "", err
	}

	resolvedTag := tag
	if plan.DryRun {
		if resolvedTag == "" {
			resolvedTag = "latest"
		}
		plan.Say("would download %s", ReleaseAssetURL(engineRepo, tag, asset))
		plan.Say("would extract %s into %s", asset, installDir)
		plan.Say("would chmod 0755 %s/%s", installDir, engineBinary)
	} else {
		if resolvedTag == "" {
			t, err := ResolveLatestTag(engineRepo)
			if err != nil {
				return "", fmt.Errorf("resolving latest engine tag: %w", err)
			}
			resolvedTag = t
		}
		tmp, err := os.MkdirTemp("", "trinity-engine-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(tmp)

		zipPath := filepath.Join(tmp, asset)
		if err := DownloadReleaseAsset(engineRepo, resolvedTag, asset, zipPath); err != nil {
			return "", err
		}
		if err := VerifyReleaseChecksum(engineRepo, resolvedTag, asset, zipPath); err != nil {
			return "", err
		}
		if uid > 0 {
			// The root-owned 0700 temp dir must be traversable by the
			// service user while the helper runs.
			if err := os.Chmod(tmp, 0755); err != nil {
				return "", fmt.Errorf("chmod %s: %w", tmp, err)
			}
			if err := plan.UnzipAs(uid, gid, zipPath, installDir, 1); err != nil {
				return "", err
			}
		} else {
			if err := UnzipInto(zipPath, installDir, 1); err != nil {
				return "", err
			}
		}
		binPath := filepath.Join(installDir, engineBinary)
		if _, err := os.Stat(binPath); err != nil {
			return "", fmt.Errorf("expected %s in release zip but it's missing; the release layout may have changed: %w", engineBinary, err)
		}
		if err := os.Chmod(binPath, 0755); err != nil {
			return "", fmt.Errorf("chmod %s: %w", binPath, err)
		}
	}

	demosTarget := filepath.Join(staticDir, "demos")
	if err := plan.MkdirAll(demosTarget, 0755); err != nil {
		return "", err
	}
	for _, sub := range []string{"baseq3", "missionpack"} {
		dir := filepath.Join(installDir, sub)
		if err := plan.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
		if err := replaceWithSymlink(plan, filepath.Join(dir, "logs"), logDir); err != nil {
			return "", err
		}
		if err := replaceWithSymlink(plan, filepath.Join(dir, "demos"), demosTarget); err != nil {
			return "", err
		}
	}
	return resolvedTag, nil
}

// replaceWithSymlink points link at target, first removing a directory
// (such as the one the engine zip unpacked) or a stale symlink there, so
// rerunning the wizard is safe.
func replaceWithSymlink(plan *Plan, link, target string) error {
	if !plan.DryRun {
		if info, err := os.Lstat(link); err == nil && info.Mode().IsDir() {
			if err := os.RemoveAll(link); err != nil {
				return fmt.Errorf("removing stale %s: %w", link, err)
			}
		} else if err == nil && info.Mode()&os.ModeSymlink != 0 {
			_ = os.Remove(link)
		}
	}
	return plan.Symlink(target, link)
}

// UnzipInto extracts every file in src into dest, preserving the
// per-file mode bits from the zip header. stripComponents drops that
// many leading path segments from each entry, like tar's
// --strip-components, which peels the engine release zips' wrapper
// directory (linux-x86_64/trinity.ded → trinity.ded). Entries with
// fewer segments are skipped, as tar does. `_helper unzip` calls it as
// the service user.
func UnzipInto(src, dest string, stripComponents int) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("opening zip %s: %w", src, err)
	}
	defer r.Close()
	for _, f := range r.File {
		name := f.Name
		if stripComponents > 0 {
			parts := strings.SplitN(name, "/", stripComponents+1)
			if len(parts) <= stripComponents {
				continue
			}
			name = parts[stripComponents]
			if name == "" {
				continue
			}
		}
		// Paths must stay inside dest.
		target := filepath.Join(dest, name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) && target != filepath.Clean(dest) {
			return fmt.Errorf("zip entry %q escapes destination", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.Mode()); err != nil {
				return fmt.Errorf("mkdir %s: %w", target, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return fmt.Errorf("creating %s: %w", target, err)
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			rc.Close()
			out.Close()
			return fmt.Errorf("writing %s: %w", target, err)
		}
		rc.Close()
		out.Close()
	}
	return nil
}
