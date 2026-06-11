package assets

import (
	"encoding/binary"
	"path/filepath"
	"testing"
)

// makeTestBSP builds a minimal valid IBSP v46 file whose shader lump
// contains the given shader names.
func makeTestBSP(shaderNames []string) string {
	header := make([]byte, bspHeaderSize)
	copy(header, bspMagic)
	binary.LittleEndian.PutUint32(header[4:], bspVersion)
	shaderData := make([]byte, len(shaderNames)*bspShaderSize)
	for i, name := range shaderNames {
		copy(shaderData[i*bspShaderSize:], name)
	}
	binary.LittleEndian.PutUint32(header[8+bspLumpShaders*8:], uint32(bspHeaderSize))
	binary.LittleEndian.PutUint32(header[8+bspLumpShaders*8+4:], uint32(len(shaderData)))
	return string(header) + string(shaderData)
}

func buildAndList(t *testing.T, mapName string, games []string, manifest *Manifest, dir string) map[string]bool {
	t.Helper()
	out := filepath.Join(dir, mapName+".pk3")
	if err := BuildMapPak(mapName, games, manifest, out); err != nil {
		t.Fatal(err)
	}
	files, err := MapPakFileSet(out)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// A map installed in baseq3 may depend on missionpack assets (it is only
// playable there) — the single map pk3 artifact must carry the union of
// dependencies resolved under every game.
func TestBuildMapPak_UnionsAcrossGames(t *testing.T) {
	dir := t.TempDir()

	skyFaces := []string{}
	skyFiles := map[string]string{}
	for _, suf := range []string{"_rt", "_lf", "_bk", "_ft", "_up", "_dn"} {
		skyFaces = append(skyFaces, "textures/skies2/env/neb"+suf)
		skyFiles["textures/skies2/env/neb"+suf+".tga"] = "tga"
	}

	bq3Pk3 := writeTestPk3(t, dir, "bq3map.pk3", map[string]string{
		"maps/czmap.bsp":         makeTestBSP([]string{"textures/base/wall", "textures/skies2/nebula2"}),
		"textures/base/wall.jpg": "jpg",
	})
	mpPk3 := writeTestPk3(t, dir, "mppak.pk3", skyFiles)

	bq3Index := map[string]string{
		"maps/czmap.bsp":         bq3Pk3,
		"textures/base/wall.jpg": bq3Pk3,
	}
	mpIndex := map[string]string{}
	for k, v := range bq3Index {
		mpIndex[k] = v
	}
	for f := range skyFiles {
		mpIndex[f] = mpPk3
	}

	manifest := &Manifest{Games: map[string]*GameManifest{
		"baseq3": {
			FileIndex:     bq3Index,
			BaselineFiles: map[string]bool{},
			Shaders:       map[string][]string{},
			ShaderFiles:   map[string]string{},
		},
		"missionpack": {
			FileIndex:     mpIndex,
			BaselineFiles: map[string]bool{},
			Shaders:       map[string][]string{"textures/skies2/nebula2": skyFaces},
			ShaderFiles:   map[string]string{},
		},
	}}

	files := buildAndList(t, "czmap", []string{"baseq3", "missionpack"}, manifest, dir)

	for _, want := range []string{"maps/czmap.bsp", "textures/base/wall.jpg"} {
		if !files[want] {
			t.Errorf("missing baseq3-resolved file %s", want)
		}
	}
	for f := range skyFiles {
		if !files[f] {
			t.Errorf("missing missionpack-resolved file %s", f)
		}
	}
}

// Baseline subtraction is per game: a file in the missionpack baseline
// but not the baseq3 one must still be bundled, or a baseq3 mount of
// the same map pk3 would lack it.
func TestBuildMapPak_BaselineSubtractionPerGame(t *testing.T) {
	dir := t.TempDir()

	pk3 := writeTestPk3(t, dir, "map.pk3", map[string]string{
		"maps/czmap.bsp":           makeTestBSP([]string{"textures/ctf2/banner"}),
		"textures/ctf2/banner.jpg": "jpg",
	})
	index := map[string]string{
		"maps/czmap.bsp":           pk3,
		"textures/ctf2/banner.jpg": pk3,
	}

	manifest := &Manifest{Games: map[string]*GameManifest{
		"baseq3": {
			FileIndex:     index,
			BaselineFiles: map[string]bool{},
			Shaders:       map[string][]string{},
			ShaderFiles:   map[string]string{},
		},
		"missionpack": {
			FileIndex:     index,
			BaselineFiles: map[string]bool{"textures/ctf2/banner.jpg": true},
			Shaders:       map[string][]string{},
			ShaderFiles:   map[string]string{},
		},
	}}

	files := buildAndList(t, "czmap", []string{"baseq3", "missionpack"}, manifest, dir)

	if !files["textures/ctf2/banner.jpg"] {
		t.Error("file in missionpack baseline only must still be bundled for baseq3 mounts")
	}
}
