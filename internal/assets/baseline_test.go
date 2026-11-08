package assets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestBuildBaseline_PakOrderPicksShaderAndGeneratesScript(t *testing.T) {
	quake3Dir := t.TempDir()
	outputDir := t.TempDir()
	bq3Dir := filepath.Join(quake3Dir, "baseq3")
	if err := os.MkdirAll(bq3Dir, 0755); err != nil {
		t.Fatal(err)
	}

	stage := func(name, image string) string {
		return name + "\n{\n\t{\n\t\tmap " + image + "\n\t}\n}\n"
	}
	writeOrderedTestPk3(t, bq3Dir, "pak0.pk3",
		[2]string{"scripts/sfx.shader", stage("textures/sfx/glow", "textures/sfx/stockglow.tga") + stage("textures/sfx/beam", "textures/sfx/stockbeam.tga")},
		[2]string{"textures/sfx/stockglow.tga", "S"},
		[2]string{"textures/sfx/stockbeam.tga", "S"},
	)
	writeOrderedTestPk3(t, bq3Dir, "aerowalk.pk3",
		[2]string{"maps/aerowalk.bsp", makeTestBSP([]string{"textures/sfx/glow", "textures/sfx/beam", "textures/aero/wall"})},
		[2]string{"scripts/aerowalk.shader", stage("textures/sfx/glow", "textures/aero/glow.tga") + stage("textures/aero/wall", "textures/aero/wall.tga")},
		[2]string{"textures/aero/glow.tga", "AG"},
		[2]string{"textures/aero/wall.tga", "AW"},
	)
	writeOrderedTestPk3(t, bq3Dir, "zen.pk3",
		[2]string{"maps/zen.bsp", makeTestBSP([]string{"textures/sfx/beam"})},
		[2]string{"scripts/zen.shader", stage("textures/sfx/beam", "textures/zen/beam.tga")},
		[2]string{"textures/zen/beam.tga", "ZB"},
	)

	if err := BuildBaseline(quake3Dir, outputDir); err != nil {
		t.Fatal(err)
	}

	read := func(mapName, path string) (string, bool) {
		pk3 := filepath.Join(outputDir, "maps", mapName+".pk3")
		files, err := MapPakFileSet(pk3)
		if err != nil {
			t.Fatal(err)
		}
		if !files[path] {
			return "", false
		}
		data, err := ReadFileFromPk3(pk3, path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data), true
	}

	aero, ok := read("aerowalk", "scripts/demobake_aerowalk.shader")
	if !ok {
		t.Fatal("aerowalk: generated shader file missing")
	}
	if strings.Contains(aero, "textures/sfx/glow") {
		t.Errorf("aerowalk: glow is won by pak0, which outranks aerowalk.pk3:\n%s", aero)
	}
	if !strings.Contains(aero, "textures/aero/wall.tga") {
		t.Errorf("aerowalk: own shader missing:\n%s", aero)
	}
	if !strings.Contains(aero, "textures/zen/beam.tga") {
		t.Errorf("aerowalk: beam is won by zen.pk3, which outranks pak0:\n%s", aero)
	}
	for _, path := range []string{"textures/aero/wall.tga", "textures/zen/beam.tga"} {
		if _, ok := read("aerowalk", path); !ok {
			t.Errorf("aerowalk: %s missing", path)
		}
	}
	if _, ok := read("aerowalk", "textures/aero/glow.tga"); ok {
		t.Error("aerowalk: the losing glow's image must not be bundled")
	}
	if _, ok := read("aerowalk", "scripts/aerowalk.shader"); ok {
		t.Error("aerowalk: source shader scripts must not be bundled")
	}

	zen, ok := read("zen", "scripts/demobake_zen.shader")
	if !ok {
		t.Fatal("zen: generated shader file missing")
	}
	if !strings.Contains(zen, "textures/zen/beam.tga") {
		t.Errorf("zen: the server's winning beam must be carried:\n%s", zen)
	}
}
