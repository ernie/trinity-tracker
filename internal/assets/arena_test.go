package assets

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// makeTestBSPWithMessage builds a minimal IBSP whose worldspawn carries the
// given "message" value.
func makeTestBSPWithMessage(message string) string {
	ent := "{\n\"classname\" \"worldspawn\"\n\"message\" \"" + message + "\"\n}\n"
	header := make([]byte, bspHeaderSize)
	copy(header, bspMagic)
	binary.LittleEndian.PutUint32(header[4:], bspVersion)
	binary.LittleEndian.PutUint32(header[8+bspLumpEntities*8:], uint32(bspHeaderSize))
	binary.LittleEndian.PutUint32(header[8+bspLumpEntities*8+4:], uint32(len(ent)))
	return string(header) + ent
}

func TestBSPWorldspawnMessage(t *testing.T) {
	bsp := makeTestBSPWithMessage("Arkinholm")
	got, err := BSPWorldspawnMessage(strings.NewReader(bsp), int64(len(bsp)))
	if err != nil {
		t.Fatal(err)
	}
	if got != "Arkinholm" {
		t.Errorf("message: got %q, want %q", got, "Arkinholm")
	}

	noMsg := makeTestBSP(nil)
	got, err = BSPWorldspawnMessage(strings.NewReader(noMsg), int64(len(noMsg)))
	if err != nil || got != "" {
		t.Errorf("no-message BSP: got %q err %v, want empty", got, err)
	}
}

func TestParseArenaLongNames(t *testing.T) {
	content := "{\n" +
		"map\t\t\"mpteam8\"\n" +
		"longname\t\"Assassin's Roost\"\n" +
		"type\t\t\"ctf oneflag harvester overload\"\n" +
		"}\n\n" +
		"{\n" +
		"map\t\t\"mpteam9\"\n" +
		"longname\t\"Tristitia\"\n" +
		"}\n"
	got := parseArenaLongNames([]byte(content))
	if got["mpteam8"] != "Assassin's Roost" {
		t.Errorf("mpteam8: got %q", got["mpteam8"])
	}
	if got["mpteam9"] != "Tristitia" {
		t.Errorf("mpteam9: got %q", got["mpteam9"])
	}
}

// A Team Arena map with no worldspawn "message" (mpteam8) must still get its
// longname from the bundled scripts/*.arena file.
func TestExtractMapLongNames_ArenaFallback(t *testing.T) {
	dir := t.TempDir()
	pk3 := writeTestPk3(t, dir, "mp.pk3", map[string]string{
		"maps/mpteam8.bsp":          makeTestBSP(nil),
		"scripts/missionpack.arena": "{\nmap \"mpteam8\"\nlongname \"Assassin's Roost\"\n}\n",
	})

	got, err := ExtractMapLongNames([]string{pk3})
	if err != nil {
		t.Fatal(err)
	}
	if got["mpteam8"].LongName != "Assassin's Roost" {
		t.Errorf("mpteam8 longname: %q", got["mpteam8"].LongName)
	}
}

// The worldspawn message is the in-game name and stays authoritative; the
// .arena longname only fills maps that lack one.
func TestExtractMapLongNames_WorldspawnWinsOverArena(t *testing.T) {
	dir := t.TempDir()
	pk3 := writeTestPk3(t, dir, "mp.pk3", map[string]string{
		"maps/q3dm6.bsp":      makeTestBSPWithMessage("The Camping Grounds"),
		"scripts/q3dm6.arena": "{\nmap \"q3dm6\"\nlongname \"Wrong Name\"\n}\n",
	})

	got, err := ExtractMapLongNames([]string{pk3})
	if err != nil {
		t.Fatal(err)
	}
	if got["q3dm6"].LongName != "The Camping Grounds" {
		t.Errorf("q3dm6 longname: got %q, want worldspawn message", got["q3dm6"].LongName)
	}
}

// One pk3 exercising each outcome: named by message, rescued by .arena, a .bsp
// with no name from any source, and an arena entry for a map that has no .bsp.
func TestMapNameSources_Report(t *testing.T) {
	dir := t.TempDir()
	pk3 := writeTestPk3(t, dir, "pak0.pk3", map[string]string{
		"maps/q3dm6.bsp":   makeTestBSPWithMessage("The Camping Grounds"),
		"maps/mpteam8.bsp": makeTestBSP(nil),
		"maps/orphan.bsp":  makeTestBSP(nil),
		"scripts/missionpack.arena": "{\nmap \"mpteam8\"\nlongname \"Assassin's Roost\"\n}\n" +
			"{\nmap \"notinstalled\"\nlongname \"Ghost\"\n}\n",
	})

	s := NewMapNameSources()
	if err := s.Collect(pk3); err != nil {
		t.Fatal(err)
	}

	resolved := s.Resolve()
	if resolved["q3dm6"].LongName != "The Camping Grounds" {
		t.Errorf("q3dm6: %q", resolved["q3dm6"].LongName)
	}
	if resolved["mpteam8"].LongName != "Assassin's Roost" {
		t.Errorf("mpteam8: %q", resolved["mpteam8"].LongName)
	}
	if _, ok := resolved["notinstalled"]; ok {
		t.Error("arena entry with no .bsp must not be emitted")
	}
	if _, ok := resolved["orphan"]; ok {
		t.Error("map with no name must not be emitted")
	}

	if filled := s.ArenaFilled(); len(filled) != 1 || filled[0] != "mpteam8" {
		t.Errorf("ArenaFilled = %v, want [mpteam8]", filled)
	}
	if un := s.Unnamed(); len(un) != 1 || un[0] != "orphan" {
		t.Errorf("Unnamed = %v, want [orphan]", un)
	}
}

// A map's .arena file can ride in a different pk3 than its .bsp. The worldspawn
// message must stay authoritative even when the conflicting .arena loads later.
func TestExtractMapLongNames_WorldspawnWinsAcrossPk3s(t *testing.T) {
	dir := t.TempDir()
	bspPk3 := writeTestPk3(t, dir, "pak0.pk3", map[string]string{
		"maps/q3dm6.bsp": makeTestBSPWithMessage("The Camping Grounds"),
	})
	arenaPk3 := writeTestPk3(t, dir, "pak1.pk3", map[string]string{
		"scripts/q3dm6.arena": "{\nmap \"q3dm6\"\nlongname \"Wrong Name\"\n}\n",
	})

	got, err := ExtractMapLongNames([]string{bspPk3, arenaPk3})
	if err != nil {
		t.Fatal(err)
	}
	if got["q3dm6"].LongName != "The Camping Grounds" {
		t.Errorf("q3dm6 longname: got %q, want worldspawn message", got["q3dm6"].LongName)
	}
}

// The inverse split: .bsp (no message) and its .arena live in separate pk3s.
func TestExtractMapLongNames_ArenaFallbackAcrossPk3s(t *testing.T) {
	dir := t.TempDir()
	bspPk3 := writeTestPk3(t, dir, "pak0.pk3", map[string]string{
		"maps/mpteam8.bsp": makeTestBSP(nil),
	})
	arenaPk3 := writeTestPk3(t, dir, "pak1.pk3", map[string]string{
		"scripts/missionpack.arena": "{\nmap \"mpteam8\"\nlongname \"Assassin's Roost\"\n}\n",
	})

	got, err := ExtractMapLongNames([]string{bspPk3, arenaPk3})
	if err != nil {
		t.Fatal(err)
	}
	if got["mpteam8"].LongName != "Assassin's Roost" {
		t.Errorf("mpteam8 longname: %q", got["mpteam8"].LongName)
	}
}

func TestExtractMapLongNames(t *testing.T) {
	dir := t.TempDir()
	pk3 := writeTestPk3(t, dir, "maps.pk3", map[string]string{
		"maps/arkinholm.bsp": makeTestBSPWithMessage("Arkinholm"),
		"maps/aerowalk.bsp":  makeTestBSPWithMessage("Aerowalk"),
		"scripts/foo.shader": "// not a bsp",
	})

	got, err := ExtractMapLongNames([]string{pk3})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(got), got)
	}
	if got["arkinholm"].LongName != "Arkinholm" {
		t.Errorf("arkinholm longname: %q", got["arkinholm"].LongName)
	}
	if got["aerowalk"].LongName != "Aerowalk" {
		t.Errorf("aerowalk longname: %q", got["aerowalk"].LongName)
	}
}
