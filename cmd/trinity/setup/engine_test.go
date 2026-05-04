package setup

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestLookupChecksum(t *testing.T) {
	manifest := `
abc123  trinity-linux-x86_64.zip
def456 *trinity-linux-arm64.zip
xyz789  some-other-file.zip
`
	cases := map[string]string{
		"trinity-linux-x86_64.zip": "abc123", // text mode (two spaces)
		"trinity-linux-arm64.zip":  "def456", // binary mode (asterisk)
		"some-other-file.zip":      "xyz789",
		"missing.zip":              "",
	}
	for asset, want := range cases {
		got := lookupChecksum(manifest, asset)
		if got != want {
			t.Errorf("lookupChecksum(%q): got %q, want %q", asset, got, want)
		}
	}
}

// TestUnzipInto_StripComponents verifies that the wrapper directory
// the trinity-engine release zips include (linux-x86_64/, etc.) gets
// stripped during extraction so the binary lands at installDir/trinity.ded
// rather than installDir/linux-x86_64/trinity.ded. Regression guard for
// a real failure that bit a v0.10.0 collector install on Debian.
func TestUnzipInto_StripComponents(t *testing.T) {
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "test.zip")
	dest := filepath.Join(tmp, "out")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	// Build a tiny zip mirroring the engine release layout (since v0.9.20):
	// one wrapper dir containing the suffix-free binary plus a subdirectory.
	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	add := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	add("linux-x86_64/trinity.ded", "fake-binary")
	add("linux-x86_64/baseq3/pak0.pk3", "fake-pak")
	add("linux-x86_64/EULA.txt", "fake-eula")
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zf.Close(); err != nil {
		t.Fatal(err)
	}

	if err := UnzipInto(zipPath, dest, 1); err != nil {
		t.Fatalf("UnzipInto: %v", err)
	}

	// After strip-1, files should be at dest/<basename> directly,
	// NOT dest/linux-x86_64/<basename>.
	wantPaths := []string{
		"trinity.ded",
		"baseq3/pak0.pk3",
		"EULA.txt",
	}
	for _, p := range wantPaths {
		if _, err := os.Stat(filepath.Join(dest, p)); err != nil {
			t.Errorf("expected %s after strip-1: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "linux-x86_64")); !os.IsNotExist(err) {
		t.Errorf("wrapper dir should NOT exist after strip-1 (err=%v)", err)
	}
}
