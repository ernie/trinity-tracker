package assets

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func pk3Names(t *testing.T, path string) map[string]bool {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	names := make(map[string]bool)
	for _, f := range r.File {
		if !f.FileInfo().IsDir() {
			names[f.Name] = true
		}
	}
	return names
}

func TestWriteMobilePak_StripsBaselineShadowedImages(t *testing.T) {
	dir := t.TempDir()
	src := writeTestPk3(t, dir, "pak8t.pk3", map[string]string{
		"models/players/doom/doom_blue.tga":  "upscaled",
		"models/players/Doom/ICON_RED.TGA":   "upscaled",
		"models/players/brandon/brandon.tga": "trinity-only",
		"maps/q3dm7.aat":                     "botnav",
		"vm/cgame.qvm":                       "qvm",
		"scripts/players.shader":             "shader",
	})
	baseline := map[string]bool{
		"models/players/doom/doom_blue.tga": true,
		"models/players/doom/icon_red.tga":  true,
		"vm/cgame.qvm":                      true,
	}

	dst := filepath.Join(dir, "pak8t-mobile.pk3")
	if err := WriteMobilePak(src, dst, baseline); err != nil {
		t.Fatal(err)
	}

	got := pk3Names(t, dst)
	want := map[string]bool{
		"models/players/brandon/brandon.tga": true,
		"vm/cgame.qvm":                       true,
		"scripts/players.shader":             true,
	}
	for name := range want {
		if !got[name] {
			t.Errorf("mobile pak missing %s", name)
		}
	}
	for name := range got {
		if !want[name] {
			t.Errorf("mobile pak should not contain %s", name)
		}
	}
}

func TestBuildBaseline_WritesMobileTrinityPak(t *testing.T) {
	quake3Dir := t.TempDir()
	outputDir := t.TempDir()
	bq3Dir := filepath.Join(quake3Dir, "baseq3")
	if err := os.MkdirAll(bq3Dir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestPk3(t, bq3Dir, "pak0.pk3", map[string]string{
		"gfx/2d/bigchars.tga": "stock",
	})
	writeTestPk3(t, bq3Dir, "pak8t.pk3", map[string]string{
		"gfx/2d/bigchars.tga":        "upscaled",
		"models/players/foo/new.tga": "trinity-only",
		"vm/cgame.qvm":               "qvm",
	})

	if err := BuildBaseline(quake3Dir, outputDir); err != nil {
		t.Fatal(err)
	}

	got := pk3Names(t, filepath.Join(outputDir, "pak8t-mobile.pk3"))
	want := map[string]bool{
		"models/players/foo/new.tga": true,
		"vm/cgame.qvm":               true,
	}
	for name := range want {
		if !got[name] {
			t.Errorf("mobile pak missing %s", name)
		}
	}
	for name := range got {
		if !want[name] {
			t.Errorf("mobile pak should not contain %s", name)
		}
	}
}
