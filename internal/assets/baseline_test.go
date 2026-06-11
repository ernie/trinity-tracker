package assets

import (
	"os"
	"path/filepath"
	"testing"
)

// Regression: a third-party map installed in baseq3/ whose sky and
// textures live in missionpack's pak0 (czq3p61ctf1) must get those
// assets in its map pk3 — baking only against the baseq3 manifest
// silently dropped them, leaving a hall-of-mirrors sky in the player.
func TestBuildBaseline_Baseq3MapGetsMissionpackAssets(t *testing.T) {
	quake3Dir := t.TempDir()
	outputDir := t.TempDir()
	bq3Dir := filepath.Join(quake3Dir, "baseq3")
	mpDir := filepath.Join(quake3Dir, "missionpack")
	for _, d := range []string{bq3Dir, mpDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	writeTestPk3(t, bq3Dir, "pak0.pk3", map[string]string{
		"gfx/2d/bigchars.tga": "tga",
	})
	writeTestPk3(t, bq3Dir, "map-czmap.pk3", map[string]string{
		"maps/czmap.bsp": makeTestBSP([]string{"textures/skies2/nebula2"}),
	})
	writeTestPk3(t, mpDir, "pak0.pk3", map[string]string{
		"scripts/sky.shader": `
textures/skies2/nebula2
{
	surfaceparm sky
	skyparms textures/skies2/env/neb - -
}
`,
		"textures/skies2/env/neb_rt.tga": "tga",
		"textures/skies2/env/neb_lf.tga": "tga",
		"textures/skies2/env/neb_bk.tga": "tga",
		"textures/skies2/env/neb_ft.tga": "tga",
		"textures/skies2/env/neb_up.tga": "tga",
		"textures/skies2/env/neb_dn.tga": "tga",
	})

	if err := BuildBaseline(quake3Dir, outputDir); err != nil {
		t.Fatal(err)
	}

	files, err := MapPakFileSet(filepath.Join(outputDir, "maps", "czmap.pk3"))
	if err != nil {
		t.Fatal(err)
	}
	for _, suf := range []string{"_rt", "_lf", "_bk", "_ft", "_up", "_dn"} {
		want := "textures/skies2/env/neb" + suf + ".tga"
		if !files[want] {
			t.Errorf("map pk3 missing missionpack skybox face %s", want)
		}
	}
}
