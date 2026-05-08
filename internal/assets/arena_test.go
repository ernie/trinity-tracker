package assets

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArenaText_OfficialBaseq3Format(t *testing.T) {
	input := `{
map     "q3dm17"
bots    "anarki angel keel"
longname "The Longest Yard"
fraglimit 30
type    "ffa tourney"
}

{
map     "q3dm6"
longname "The Camping Grounds"
type    "single ffa team"
fraglimit 20
}`

	got, err := ParseArenaText(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseArenaText: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].Map != "q3dm17" {
		t.Errorf("entry 0 map: want q3dm17, got %q", got[0].Map)
	}
	if got[0].LongName != "The Longest Yard" {
		t.Errorf("entry 0 longname: want %q, got %q", "The Longest Yard", got[0].LongName)
	}
	if got[0].Type != "ffa tourney" {
		t.Errorf("entry 0 type: want %q, got %q", "ffa tourney", got[0].Type)
	}
	if got[0].FragLimit != 30 {
		t.Errorf("entry 0 fraglimit: want 30, got %d", got[0].FragLimit)
	}
	if got[1].Map != "q3dm6" {
		t.Errorf("entry 1 map: want q3dm6, got %q", got[1].Map)
	}
}

func TestParseArenaText_CommunityFormatWithAuthor(t *testing.T) {
	input := `{
map "mIKEctf3"
longname "My iron lung"
type "cctf ctf oneflag harvester overload"
mod "threewave"
author "mIKE"
quote "...3...2...1... Capture the flaaag"
}`

	got, err := ParseArenaText(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseArenaText: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].Author != "mIKE" {
		t.Errorf("author: want mIKE, got %q", got[0].Author)
	}
}

func TestParseArenaText_HandlesEmptyAndUnknownKeys(t *testing.T) {
	input := `

{
	map "test1"
	longname "Test One"
	unknown_key "ignored"
}
`
	got, err := ParseArenaText(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Map != "test1" || got[0].LongName != "Test One" {
		t.Fatalf("got: %+v", got)
	}
}

// writeTestPk3 creates a temporary pk3 (zip) file with the given path/content
// pairs and returns its filesystem path.
func writeTestPk3(t *testing.T, dir, name string, files map[string]string) string {
	t.Helper()
	pk3Path := filepath.Join(dir, name)
	f, err := os.Create(pk3Path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for path, content := range files {
		fw, err := w.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(fw, bytes.NewReader([]byte(content))); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return pk3Path
}

func TestExtractArenas_MultiplePk3sWithOverrides(t *testing.T) {
	dir := t.TempDir()

	pakBase := writeTestPk3(t, dir, "pak0.pk3", map[string]string{
		"scripts/arenas.txt": `{
map "q3dm17"
longname "The Longest Yard"
type "ffa"
}
{
map "q3dm6"
longname "The Camping Grounds"
type "ffa team"
}`,
	})

	pakCommunity := writeTestPk3(t, dir, "z_community.pk3", map[string]string{
		"scripts/mymap.arena": `{
map "mymap"
longname "My Map"
type "ctf"
}`,
	})

	got, err := ExtractArenas([]string{pakBase, pakCommunity})
	if err != nil {
		t.Fatalf("ExtractArenas: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d: %+v", len(got), got)
	}
	if got["q3dm17"].LongName != "The Longest Yard" {
		t.Errorf("q3dm17 longname: %q", got["q3dm17"].LongName)
	}
	if got["mymap"].LongName != "My Map" {
		t.Errorf("mymap longname: %q", got["mymap"].LongName)
	}
}
