package setup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const trackerRepo = "ernie/trinity-tracker"

// trackerAsset is the release-asset name for the given GOARCH (or the
// host arch if goarch == ""). Returns an error for arches with no
// published build.
func trackerAsset(goarch string) (string, error) {
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	switch goarch {
	case "amd64", "arm64", "arm":
		return fmt.Sprintf("trinity-linux-%s.tar.gz", goarch), nil
	}
	return "", fmt.Errorf("unsupported arch %s; see https://github.com/%s/releases", goarch, trackerRepo)
}

// TrackerStage is the result of staging a tracker release for install:
// the new binary is at BinPath; if the release also bundles a web/
// directory it's at WebDir (otherwise ""). ResolvedTag is whatever
// tag "latest" resolved to (or the explicit tag passed in).
type TrackerStage struct {
	BinPath     string
	WebDir      string
	ResolvedTag string
}

// DownloadTrackerRelease fetches the trinity-tracker release tarball
// for tag (or latest if tag == ""), verifies it against the release's
// sha256sums.txt, extracts into stageDir, and returns paths to the
// trinity binary and the bundled web/ directory (when present).
//
// The web/ directory is the static SPA bundle Vite builds; release
// pipelines that drop it into the tarball alongside the binary make
// it possible to refresh hub web assets in the same step as the
// binary swap.
func DownloadTrackerRelease(tag, stageDir string) (*TrackerStage, error) {
	asset, err := trackerAsset("")
	if err != nil {
		return nil, err
	}
	resolvedTag := tag
	if resolvedTag == "" {
		resolvedTag, err = ResolveLatestTag(trackerRepo)
		if err != nil {
			return nil, fmt.Errorf("resolving latest tracker tag: %w", err)
		}
	}
	if err := os.MkdirAll(stageDir, 0755); err != nil {
		return nil, err
	}
	tarballPath := filepath.Join(stageDir, asset)
	if err := DownloadReleaseAsset(trackerRepo, resolvedTag, asset, tarballPath); err != nil {
		return nil, err
	}
	if err := VerifyReleaseChecksum(trackerRepo, resolvedTag, asset, tarballPath); err != nil {
		return nil, err
	}
	extractDir := filepath.Join(stageDir, "extracted")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return nil, err
	}
	if err := untarStripOne(tarballPath, extractDir); err != nil {
		return nil, fmt.Errorf("extracting %s: %w", tarballPath, err)
	}
	_ = os.Remove(tarballPath)
	binPath := filepath.Join(extractDir, "trinity")
	if _, err := os.Stat(binPath); err != nil {
		return nil, fmt.Errorf("expected trinity binary in tarball: %w", err)
	}
	stage := &TrackerStage{BinPath: binPath, ResolvedTag: resolvedTag}
	webDir := filepath.Join(extractDir, "web")
	if info, err := os.Stat(webDir); err == nil && info.IsDir() {
		stage.WebDir = webDir
	}
	return stage, nil
}

// untarStripOne extracts a .tar.gz into dest, dropping the leading
// path segment from each entry — matches `tar --strip-components=1`,
// the convention every trinity-* release tarball uses (top-level dir
// named after the asset).
func untarStripOne(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		parts := strings.SplitN(hdr.Name, "/", 2)
		if len(parts) < 2 || parts[1] == "" {
			continue
		}
		target := filepath.Join(dest, parts[1])
		// Reject path traversal — entries must stay inside dest.
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("tar entry %q escapes destination", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)|0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		case tar.TypeSymlink:
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		}
	}
}
