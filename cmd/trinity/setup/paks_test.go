package setup

import (
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
