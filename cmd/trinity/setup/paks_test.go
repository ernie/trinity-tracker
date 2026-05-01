package setup

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestMissingPaks(t *testing.T) {
	dir := t.TempDir()
	mod := "baseq3"
	if err := os.MkdirAll(filepath.Join(dir, mod), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Drop pak0 + pak3 in place; pak1/pak2 stay missing.
	for _, n := range []string{"pak0.pk3", "pak3.pk3"} {
		if err := os.WriteFile(filepath.Join(dir, mod, n), []byte("PK\x03\x04"), 0644); err != nil {
			t.Fatalf("write %s: %v", n, err)
		}
	}
	got := missingPaks(dir, mod, []string{"pak0.pk3", "pak1.pk3", "pak2.pk3", "pak3.pk3"})
	want := []string{"pak1.pk3", "pak2.pk3"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestAnyPatchMissing(t *testing.T) {
	cases := []struct {
		in   []string
		want bool
	}{
		{nil, false},
		{[]string{"pak0.pk3"}, false},
		{[]string{"pak0.pk3", "pak3.pk3"}, true},
		{[]string{"pak1.pk3"}, true},
	}
	for _, c := range cases {
		if got := anyPatchMissing(c.in); got != c.want {
			t.Errorf("anyPatchMissing(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIsPlausiblePak(t *testing.T) {
	dir := t.TempDir()
	// Real-shaped: zip magic.
	zipPath := filepath.Join(dir, "real.pk3")
	if err := os.WriteFile(zipPath, []byte("PK\x03\x04rest of zip"), 0644); err != nil {
		t.Fatalf("write zip: %v", err)
	}
	if !isPlausiblePak(zipPath) {
		t.Errorf("expected real.pk3 (zip-magic) to pass")
	}
	// Wrong magic.
	textPath := filepath.Join(dir, "bogus.pk3")
	if err := os.WriteFile(textPath, []byte("not a zip"), 0644); err != nil {
		t.Fatalf("write text: %v", err)
	}
	if isPlausiblePak(textPath) {
		t.Errorf("expected bogus.pk3 to fail magic check")
	}
	// Missing file.
	if isPlausiblePak(filepath.Join(dir, "ghost.pk3")) {
		t.Errorf("expected missing file to fail")
	}
	// Too short to read 4 bytes.
	short := filepath.Join(dir, "short.pk3")
	if err := os.WriteFile(short, []byte("PK"), 0644); err != nil {
		t.Fatalf("write short: %v", err)
	}
	if isPlausiblePak(short) {
		t.Errorf("expected short file to fail")
	}
}

func TestHubURL(t *testing.T) {
	if got := hubURL("", "/x"); got != "" {
		t.Errorf("empty host: got %q, want empty", got)
	}
	if got := hubURL("trinity.example.com", "/downloads/foo.zip"); got != "https://trinity.example.com/downloads/foo.zip" {
		t.Errorf("got %q", got)
	}
}

// makeTestZip builds an in-memory zip with the given file map (path
// inside zip → bytes) and writes it to dest.
func makeTestZip(t *testing.T, dest string, files map[string][]byte) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zw: %v", err)
	}
	if err := os.WriteFile(dest, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write zip: %v", err)
	}
}

func TestFetchAndExtractMods_LocalSource(t *testing.T) {
	root := t.TempDir()
	staticDir := filepath.Join(root, "web")
	q3Dir := filepath.Join(root, "quake3")
	dlDir := filepath.Join(staticDir, "downloads")
	if err := os.MkdirAll(dlDir, 0755); err != nil {
		t.Fatal(err)
	}
	makeTestZip(t, filepath.Join(dlDir, "patch.zip"), map[string][]byte{
		"baseq3/pak1.pk3":      []byte("p1"),
		"baseq3/pak2.pk3":      []byte("p2"),
		"missionpack/pak1.pk3": []byte("mp1"),
		"otherdir/ignored.pk3": []byte("nope"),
	})
	opts := PakStepOptions{
		Quake3Dir: q3Dir,
		StaticDir: staticDir,
	}
	if err := fetchAndExtractMods(opts, io.Discard, "patch.zip", []string{"baseq3", "missionpack"}); err != nil {
		t.Fatalf("fetchAndExtractMods: %v", err)
	}
	for _, want := range []string{"baseq3/pak1.pk3", "baseq3/pak2.pk3", "missionpack/pak1.pk3"} {
		if _, err := os.Stat(filepath.Join(q3Dir, want)); err != nil {
			t.Errorf("expected %s to exist: %v", want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(q3Dir, "otherdir", "ignored.pk3")); !os.IsNotExist(err) {
		t.Errorf("entries outside allowed mods should not be extracted (err=%v)", err)
	}
}

func TestFetchAndExtractMods_RestrictMods(t *testing.T) {
	root := t.TempDir()
	staticDir := filepath.Join(root, "web")
	q3Dir := filepath.Join(root, "quake3")
	dlDir := filepath.Join(staticDir, "downloads")
	if err := os.MkdirAll(dlDir, 0755); err != nil {
		t.Fatal(err)
	}
	makeTestZip(t, filepath.Join(dlDir, "hqq.zip"), map[string][]byte{
		"baseq3/pak9hqq.pk3":   []byte("hqq"),
		"missionpack/snuck.pk3": []byte("nope"),
	})
	opts := PakStepOptions{Quake3Dir: q3Dir, StaticDir: staticDir}
	if err := fetchAndExtractMods(opts, io.Discard, "hqq.zip", []string{"baseq3"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(q3Dir, "baseq3/pak9hqq.pk3")); err != nil {
		t.Errorf("baseq3 entry should be extracted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(q3Dir, "missionpack/snuck.pk3")); !os.IsNotExist(err) {
		t.Errorf("missionpack entry should be skipped (err=%v)", err)
	}
}

func TestFetchAndExtractMods_PreservesExistingFiles(t *testing.T) {
	root := t.TempDir()
	staticDir := filepath.Join(root, "web")
	q3Dir := filepath.Join(root, "quake3")
	if err := os.MkdirAll(filepath.Join(q3Dir, "baseq3"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(staticDir, "downloads"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(q3Dir, "baseq3/pak1.pk3"), []byte("operator-curated"), 0644); err != nil {
		t.Fatal(err)
	}
	makeTestZip(t, filepath.Join(staticDir, "downloads", "patch.zip"), map[string][]byte{
		"baseq3/pak1.pk3": []byte("from-zip"),
	})
	opts := PakStepOptions{Quake3Dir: q3Dir, StaticDir: staticDir}
	if err := fetchAndExtractMods(opts, io.Discard, "patch.zip", []string{"baseq3"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(q3Dir, "baseq3/pak1.pk3"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "operator-curated" {
		t.Errorf("operator file overwritten: got %q", got)
	}
}

func TestFetchAndExtractMods_StripsSingleWrapperDir(t *testing.T) {
	root := t.TempDir()
	staticDir := filepath.Join(root, "web")
	q3Dir := filepath.Join(root, "quake3")
	if err := os.MkdirAll(filepath.Join(staticDir, "downloads"), 0755); err != nil {
		t.Fatal(err)
	}
	// Real-world quake3-1.32-pk3s.zip wraps everything in
	// quake3-latest-pk3s/. Same shape exercised here.
	makeTestZip(t, filepath.Join(staticDir, "downloads", "patch.zip"), map[string][]byte{
		"quake3-latest-pk3s/baseq3/pak1.pk3":      []byte("p1"),
		"quake3-latest-pk3s/baseq3/pak8.pk3":      []byte("p8"),
		"quake3-latest-pk3s/missionpack/pak1.pk3": []byte("mp1"),
	})
	opts := PakStepOptions{Quake3Dir: q3Dir, StaticDir: staticDir}
	if err := fetchAndExtractMods(opts, io.Discard, "patch.zip", []string{"baseq3", "missionpack"}); err != nil {
		t.Fatalf("fetchAndExtractMods: %v", err)
	}
	for _, want := range []string{"baseq3/pak1.pk3", "baseq3/pak8.pk3", "missionpack/pak1.pk3"} {
		if _, err := os.Stat(filepath.Join(q3Dir, want)); err != nil {
			t.Errorf("expected %s to exist after wrapper strip: %v", want, err)
		}
	}
}

func TestFetchAndExtractMods_RejectsZipSlip(t *testing.T) {
	root := t.TempDir()
	staticDir := filepath.Join(root, "web")
	q3Dir := filepath.Join(root, "quake3")
	if err := os.MkdirAll(filepath.Join(staticDir, "downloads"), 0755); err != nil {
		t.Fatal(err)
	}
	makeTestZip(t, filepath.Join(staticDir, "downloads", "evil.zip"), map[string][]byte{
		"baseq3/../escaped.pk3": []byte("pwn"),
	})
	opts := PakStepOptions{Quake3Dir: q3Dir, StaticDir: staticDir}
	if err := fetchAndExtractMods(opts, io.Discard, "evil.zip", []string{"baseq3"}); err != nil {
		t.Fatalf("expected silent skip, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "quake3", "escaped.pk3")); !os.IsNotExist(err) {
		t.Errorf("zip-slip entry should not have been extracted (err=%v)", err)
	}
}

func TestResolveZip_HTTPFallback(t *testing.T) {
	root := t.TempDir()
	q3Dir := filepath.Join(root, "quake3")
	// Build a zip in memory and serve it from a fake hub.
	var zipBytes bytes.Buffer
	zw := zip.NewWriter(&zipBytes)
	w, _ := zw.Create("baseq3/pak1.pk3")
	_, _ = w.Write([]byte("via-http"))
	_ = zw.Close()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/downloads/patch.zip" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(zipBytes.Bytes())
	}))
	defer srv.Close()

	// resolveZip builds https://<host>/downloads/<name>; httptest's TLS
	// server has a self-signed cert, so swap http.DefaultTransport for
	// the test's permissive client.
	prevTransport := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	defer func() { http.DefaultTransport = prevTransport }()

	// HubHost has to match the test server's host.
	hostURL := srv.URL // https://127.0.0.1:PORT
	host := hostURL[len("https://"):]
	opts := PakStepOptions{
		Quake3Dir: q3Dir,
		HubHost:   host,
		// StaticDir empty → forces HTTP path.
	}
	if err := fetchAndExtractMods(opts, io.Discard, "patch.zip", []string{"baseq3"}); err != nil {
		t.Fatalf("fetchAndExtractMods: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(q3Dir, "baseq3/pak1.pk3"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "via-http" {
		t.Errorf("got %q", got)
	}
}

func TestResolveZip_NoSourceConfigured(t *testing.T) {
	opts := PakStepOptions{Quake3Dir: t.TempDir()}
	_, _, err := resolveZip(opts, io.Discard, "x.zip")
	if err == nil {
		t.Errorf("expected error when no Cwd, StaticDir, or HubHost configured")
	}
}

func TestResolveZip_CwdMirrorsToStaticDir(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "install")
	staticDir := filepath.Join(root, "web")
	q3Dir := filepath.Join(root, "quake3")
	if err := os.MkdirAll(cwd, 0755); err != nil {
		t.Fatal(err)
	}
	makeTestZip(t, filepath.Join(cwd, "patch.zip"), map[string][]byte{
		"baseq3/pak1.pk3": []byte("p1"),
	})
	opts := PakStepOptions{Quake3Dir: q3Dir, StaticDir: staticDir, Cwd: cwd}
	if err := fetchAndExtractMods(opts, io.Discard, "patch.zip", []string{"baseq3"}); err != nil {
		t.Fatal(err)
	}
	// File extracted into the q3 tree.
	if _, err := os.Stat(filepath.Join(q3Dir, "baseq3/pak1.pk3")); err != nil {
		t.Errorf("expected extraction: %v", err)
	}
	// AND mirrored into the hub's downloads dir for future collectors.
	mirrored := filepath.Join(staticDir, "downloads", "patch.zip")
	if _, err := os.Stat(mirrored); err != nil {
		t.Errorf("expected mirror to %s: %v", mirrored, err)
	}
}

func TestResolveZip_CwdNoStaticDirSkipsMirror(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "install")
	q3Dir := filepath.Join(root, "quake3")
	if err := os.MkdirAll(cwd, 0755); err != nil {
		t.Fatal(err)
	}
	makeTestZip(t, filepath.Join(cwd, "patch.zip"), map[string][]byte{
		"baseq3/pak1.pk3": []byte("p1"),
	})
	opts := PakStepOptions{Quake3Dir: q3Dir, Cwd: cwd}
	if err := fetchAndExtractMods(opts, io.Discard, "patch.zip", []string{"baseq3"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(q3Dir, "baseq3/pak1.pk3")); err != nil {
		t.Errorf("expected extraction: %v", err)
	}
}
