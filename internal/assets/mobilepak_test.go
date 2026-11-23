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

func TestRefreshMobilePak_UsesTheBakedBaseline(t *testing.T) {
	dir := t.TempDir()
	writeTestPk3(t, dir, "baseq3.pk3", map[string]string{
		"models/players/doom/doom_blue.tga": "stock",
	})
	src := writeTestPk3(t, dir, "pak8t.pk3", map[string]string{
		"models/players/doom/doom_blue.tga":     "upscaled",
		"models/players/goose/icon_default.tga": "new",
		"vm/cgame.qvm":                          "qvm",
	})

	out, err := RefreshMobilePak(dir, "baseq3", src)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "pak8t-mobile.pk3"); out != want {
		t.Fatalf("RefreshMobilePak wrote %s, want %s", out, want)
	}
	got := pk3Names(t, out)
	if got["models/players/doom/doom_blue.tga"] || !got["models/players/goose/icon_default.tga"] || !got["vm/cgame.qvm"] {
		t.Fatalf("mobile pak has %v", got)
	}
	if _, err := os.Stat(out + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file left behind: %v", err)
	}
}

func TestRefreshMobilePak_NeedsABakedBaseline(t *testing.T) {
	dir := t.TempDir()
	src := writeTestPk3(t, dir, "pak8t.pk3", map[string]string{"vm/cgame.qvm": "qvm"})
	if _, err := RefreshMobilePak(dir, "baseq3", src); err == nil {
		t.Fatal("refreshed without a baked baseq3.pk3")
	}
}

func TestRefreshMobilePaks_CoversEachGame(t *testing.T) {
	q3 := t.TempDir()
	demo := t.TempDir()
	for _, d := range []string{"baseq3", "missionpack"} {
		if err := os.MkdirAll(filepath.Join(q3, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeTestPk3(t, filepath.Join(q3, "baseq3"), "pak0.pk3", map[string]string{"vm/cgame.qvm": "stock"})
	writeTestPk3(t, filepath.Join(q3, "baseq3"), "pak8t.pk3", map[string]string{"vm/cgame.qvm": "trinity"})
	writeTestPk3(t, filepath.Join(q3, "missionpack"), "pak3t.pk3", map[string]string{"vm/ui.qvm": "trinity"})
	writeTestPk3(t, demo, "baseq3.pk3", map[string]string{"gfx/2d/crosshaira.tga": "stock"})
	writeTestPk3(t, demo, "missionpack.pk3", map[string]string{"gfx/2d/crosshaira.tga": "stock"})

	written, err := RefreshMobilePaks(q3, demo)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(demo, "pak8t-mobile.pk3"), filepath.Join(demo, "pak3t-mobile.pk3")}
	if len(written) != len(want) || written[0] != want[0] || written[1] != want[1] {
		t.Fatalf("RefreshMobilePaks wrote %v, want %v", written, want)
	}
}
