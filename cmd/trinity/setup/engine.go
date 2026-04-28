package setup

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const engineRepo = "ernie/trinity-engine"

// engineAsset returns the release asset name and the binary name
// inside the zip for the given GOARCH (or the current host arch if
// goarch is empty). Returns an error for arches with no published
// build.
func engineAsset(goarch string) (asset, binary string, err error) {
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	switch goarch {
	case "amd64":
		return "trinity-linux-x86_64.zip", "trinity.ded.x86_64", nil
	case "arm64":
		return "trinity-linux-arm64.zip", "trinity.ded.aarch64", nil
	case "arm":
		return "trinity-linux-armv7.zip", "trinity.ded.armv7l", nil
	case "386":
		return "trinity-linux-x86.zip", "trinity.ded.x86", nil
	}
	return "", "", fmt.Errorf("unsupported arch %s; see https://github.com/%s/releases", goarch, engineRepo)
}

// InstallEngine downloads the latest trinity-engine release into
// installDir. Steps:
//
//  1. Fetch trinity-linux-<arch>.zip from GitHub releases (always
//     the latest release — Trinity moves fast and old engines are
//     not supported against current hubs).
//  2. Extract into installDir.
//  3. Symlink installDir/trinity.ded → arch-specific binary so the
//     systemd unit's ExecStart works regardless of host arch.
//  4. Replace baseq3/logs and missionpack/logs with symlinks to logDir
//     so the q3 server's per-server logs land where the collector tails.
//
// Caller is responsible for chowning the install dir to the service
// user once everything's in place. In dry-run mode the download and
// extraction are skipped; only the planned filesystem effects print.
func InstallEngine(plan *Plan, installDir, logDir string) error {
	asset, binary, err := engineAsset("")
	if err != nil {
		return err
	}
	if err := plan.MkdirAll(installDir, 0755); err != nil {
		return err
	}

	if plan.DryRun {
		plan.say("would download https://github.com/%s/releases/latest/download/%s", engineRepo, asset)
		plan.say("would extract %s into %s", asset, installDir)
		plan.say("would chmod 0755 %s/%s", installDir, binary)
	} else {
		tmp, err := os.MkdirTemp("", "trinity-engine-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		zipPath := filepath.Join(tmp, asset)
		if err := downloadEngineAsset(asset, zipPath); err != nil {
			return err
		}
		if err := verifyEngineChecksum(asset, zipPath); err != nil {
			return err
		}
		if err := unzipInto(zipPath, installDir); err != nil {
			return err
		}
		binPath := filepath.Join(installDir, binary)
		if _, err := os.Stat(binPath); err != nil {
			return fmt.Errorf("expected %s in release zip but it's missing — release shape may have changed: %w", binary, err)
		}
		if err := os.Chmod(binPath, 0755); err != nil {
			return fmt.Errorf("chmod %s: %w", binPath, err)
		}
	}

	link := filepath.Join(installDir, "trinity.ded")
	if err := plan.Symlink(binary, link); err != nil {
		return err
	}

	for _, sub := range []string{"baseq3", "missionpack"} {
		dir := filepath.Join(installDir, sub)
		if err := plan.MkdirAll(dir, 0755); err != nil {
			return err
		}
		logsLink := filepath.Join(dir, "logs")
		if !plan.DryRun {
			// Replace a real directory written by the unzip; leave existing
			// symlinks alone (idempotent re-runs) and never destroy
			// operator content.
			if info, err := os.Lstat(logsLink); err == nil && info.Mode().IsDir() {
				if err := os.RemoveAll(logsLink); err != nil {
					return fmt.Errorf("removing stale %s: %w", logsLink, err)
				}
			} else if err == nil && info.Mode()&os.ModeSymlink != 0 {
				_ = os.Remove(logsLink)
			}
		}
		if err := plan.Symlink(logDir, logsLink); err != nil {
			return err
		}
	}
	return nil
}

// downloadEngineAsset fetches the latest release asset to dest.
// Prefers `gh` if available (handles auth + rate limits gracefully),
// falls back to HTTPS via net/http for the common no-tooling case.
// Always pulls the latest release — old engines are not supported
// against current hubs.
func downloadEngineAsset(asset, dest string) error {
	if path, err := exec.LookPath("gh"); err == nil {
		cmd := exec.Command(path, "release", "download",
			"--repo", engineRepo,
			"--pattern", asset,
			"--dir", filepath.Dir(dest),
			"--clobber",
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			return nil
		}
		// Fall through to HTTPS — gh may be installed but unauthenticated.
	}

	url := fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", engineRepo, asset)
	fmt.Printf("Downloading %s ...\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("downloading %s: HTTP %d", url, resp.StatusCode)
	}
	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dest, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("writing %s: %w", dest, err)
	}
	return nil
}

// verifyEngineChecksum fetches sha256sums.txt from the same release
// and checks that the local zip's SHA256 matches the manifest's
// entry for this asset. Hard-fails if the manifest is absent
// (operator must upgrade to a release that publishes one) or if
// the hash mismatches (corruption, replaced asset, MITM).
func verifyEngineChecksum(asset, zipPath string) error {
	url := fmt.Sprintf("https://github.com/%s/releases/latest/download/sha256sums.txt", engineRepo)
	fmt.Printf("Verifying checksum against %s ...\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("engine release does not publish sha256sums.txt — upgrade to a newer trinity-engine release")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading %s: %w", url, err)
	}

	expected := lookupChecksum(string(body), asset)
	if expected == "" {
		return fmt.Errorf("%s not listed in sha256sums.txt — engine release shape may have changed", asset)
	}

	f, err := os.Open(zipPath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", zipPath, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hashing %s: %w", zipPath, err)
	}
	actual := hex.EncodeToString(h.Sum(nil))

	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("checksum mismatch for %s\n  expected %s\n  got      %s", asset, expected, actual)
	}
	return nil
}

// lookupChecksum scans sha256sum-formatted manifest text for the
// entry matching asset. Handles both `<hash>  <name>` (text mode)
// and `<hash> *<name>` (binary mode) variants.
func lookupChecksum(manifest, asset string) string {
	for _, line := range strings.Split(manifest, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		hash, name := fields[0], strings.TrimPrefix(fields[1], "*")
		if name == asset {
			return hash
		}
	}
	return ""
}

// unzipInto extracts every file in src into dest, preserving the
// per-file mode bits from the zip header.
func unzipInto(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("opening zip %s: %w", src, err)
	}
	defer r.Close()
	for _, f := range r.File {
		// Reject zip-slip — paths must stay inside dest.
		target := filepath.Join(dest, f.Name)
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
