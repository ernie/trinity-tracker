package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// ResolveLatestTag returns the tag GitHub's `releases/latest` redirect
// resolves to (e.g. "v0.10.1"), read from the Location header of a
// HEAD request, which needs no auth.
func ResolveLatestTag(repo string) (string, error) {
	url := fmt.Sprintf("https://github.com/%s/releases/latest", repo)
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HEAD %s: %w", url, err)
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 3 {
		return "", fmt.Errorf("HEAD %s: expected redirect, got %d", url, resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	i := strings.LastIndex(loc, "/tag/")
	if i < 0 {
		return "", fmt.Errorf("unexpected redirect %q", loc)
	}
	return loc[i+len("/tag/"):], nil
}

// ReleaseAssetURL returns the browser download URL for a release asset.
// Pass tag = "" to mean latest.
func ReleaseAssetURL(repo, tag, asset string) string {
	if tag == "" {
		return fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", repo, asset)
	}
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, tag, asset)
}

// DownloadReleaseAsset fetches an asset by tag (or "latest") to dest
// over HTTPS.
func DownloadReleaseAsset(repo, tag, asset, dest string) error {
	url := ReleaseAssetURL(repo, tag, asset)
	return DownloadURL(url, dest)
}

// DownloadURL HTTPS-GETs url and writes the body to dest. The file's
// owner and mode come from the calling process and its umask, so
// `_helper download` runs it as the service user for service-owned files.
func DownloadURL(url, dest string) error {
	fmt.Printf("Downloading %s…\n", url)
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

// VerifyReleaseChecksum fetches the release's sha256sums.txt and
// confirms localPath's hash matches the manifest's entry for asset.
// It fails when the manifest is missing or the hash differs.
func VerifyReleaseChecksum(repo, tag, asset, localPath string) error {
	url := ReleaseAssetURL(repo, tag, "sha256sums.txt")
	fmt.Printf("Verifying checksum against %s…\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%s release does not publish sha256sums.txt; pin a newer --*-tag", repo)
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
		return fmt.Errorf("%s not listed in sha256sums.txt; the release layout may have changed", asset)
	}
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", localPath, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hashing %s: %w", localPath, err)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("checksum mismatch for %s\n  expected %s\n  got      %s", asset, expected, actual)
	}
	return nil
}

// FetchExpectedChecksum fetches sha256sums.txt from a release and
// returns the expected hex SHA-256 for asset. Returns ("", nil) when
// the manifest exists but doesn't list the asset (release predates it).
func FetchExpectedChecksum(repo, tag, asset string) (string, error) {
	url := ReleaseAssetURL(repo, tag, "sha256sums.txt")
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", url, err)
	}
	return lookupChecksum(string(body), asset), nil
}

// HashFile returns the hex-encoded SHA-256 of path.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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
