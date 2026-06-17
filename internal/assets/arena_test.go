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
