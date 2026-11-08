package assets

import (
	"archive/zip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeOrderedTestPk3 writes entries in the given order, since the engine's
// shader lookup depends on a pk3's file order.
func writeOrderedTestPk3(t *testing.T, dir, name string, entries ...[2]string) string {
	t.Helper()
	pk3Path := filepath.Join(dir, name)
	f, err := os.Create(pk3Path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for _, e := range entries {
		fw, err := w.Create(e[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(e[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return pk3Path
}

func TestCollectPk3sInDir_EngineOrder(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"zen.pk3", "pak0.pk3", "map_cpm1.pk3", "Bal3dm2.pk3", "mapz.pk3", "aerowalk.pk3", "b0_beta6.pk3", "pak8t.pk3", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "old"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "old", "stale.pk3"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, p := range CollectPk3sInDir(dir) {
		got = append(got, filepath.Base(p))
	}
	want := []string{"aerowalk.pk3", "b0_beta6.pk3", "Bal3dm2.pk3", "mapz.pk3", "map_cpm1.pk3", "pak0.pk3", "pak8t.pk3", "zen.pk3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestBuildShaderIndex_HighestPriorityPakWins(t *testing.T) {
	dir := t.TempDir()
	low := writeOrderedTestPk3(t, dir, "a.pk3", [2]string{"scripts/low.shader", "textures/x/s\n{\n\t{\n\t\tmap textures/x/low.tga\n\t}\n}\n"})
	high := writeOrderedTestPk3(t, dir, "b.pk3", [2]string{"scripts/high.shader", "textures/x/s\n{\n\t{\n\t\tmap textures/x/high.tga\n\t}\n}\n"})

	idx, err := BuildShaderIndex([]string{low, high})
	if err != nil {
		t.Fatal(err)
	}
	e := idx["textures/x/s"]
	if e.Pak != high || e.File != "scripts/high.shader" {
		t.Errorf("winner = %s %s, want the higher-priority pak", e.Pak, e.File)
	}
	if !reflect.DeepEqual(e.Textures, []string{"textures/x/high.tga"}) {
		t.Errorf("textures = %v", e.Textures)
	}
}

func TestBuildShaderIndex_FirstFileInPakWins(t *testing.T) {
	dir := t.TempDir()
	pak := writeOrderedTestPk3(t, dir, "pack.pk3",
		[2]string{"scripts/zfirst.shader", "textures/x/s\n{\n\t{\n\t\tmap textures/x/first.tga\n\t}\n}\n"},
		[2]string{"scripts/asecond.shader", "textures/x/s\n{\n\t{\n\t\tmap textures/x/second.tga\n\t}\n}\n"},
	)
	idx, err := BuildShaderIndex([]string{pak})
	if err != nil {
		t.Fatal(err)
	}
	if got := idx["textures/x/s"].File; got != "scripts/zfirst.shader" {
		t.Errorf("winner file = %s, want scripts/zfirst.shader", got)
	}
}

func TestBuildShaderIndex_LastDefinitionInFileWins(t *testing.T) {
	dir := t.TempDir()
	pak := writeOrderedTestPk3(t, dir, "pack.pk3",
		[2]string{"scripts/dup.shader", "textures/x/s\n{\n\t{\n\t\tmap textures/x/first.tga\n\t}\n}\ntextures/x/s\n{\n\t{\n\t\tmap textures/x/last.tga\n\t}\n}\n"},
	)
	idx, err := BuildShaderIndex([]string{pak})
	if err != nil {
		t.Fatal(err)
	}
	if got := idx["textures/x/s"].Textures; !reflect.DeepEqual(got, []string{"textures/x/last.tga"}) {
		t.Errorf("textures = %v, want the last definition's", got)
	}
}

func TestBuildShaderIndex_ReadsOnlyHighestCopyOfScript(t *testing.T) {
	dir := t.TempDir()
	low := writeOrderedTestPk3(t, dir, "a.pk3", [2]string{"scripts/common.shader", "textures/x/gone\n{\n}\ntextures/x/kept\n{\n}\n"})
	high := writeOrderedTestPk3(t, dir, "b.pk3", [2]string{"scripts/common.shader", "textures/x/kept\n{\n}\n"})

	idx, err := BuildShaderIndex([]string{low, high})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx["textures/x/gone"]; ok {
		t.Error("a shadowed copy of a script must not contribute definitions")
	}
	if idx["textures/x/kept"].Pak != high {
		t.Error("kept should come from the higher-priority copy")
	}
}

func TestBuildShaderIndex_KeepsDefinitionText(t *testing.T) {
	dir := t.TempDir()
	body := "textures/x/s\n{\n\tsurfaceparm nomarks // trailing note\n\t{ map textures/x/a.tga }\n\t/* block { } comment */\n}"
	pak := writeOrderedTestPk3(t, dir, "pack.pk3", [2]string{"scripts/x.shader", "// header\n" + body + "\n\ntextures/x/next\n{\n}\n"})
	idx, err := BuildShaderIndex([]string{pak})
	if err != nil {
		t.Fatal(err)
	}
	if got := idx["textures/x/s"].Text; got != body {
		t.Errorf("text =\n%q\nwant\n%q", got, body)
	}
	if got := idx["textures/x/s"].Textures; !reflect.DeepEqual(got, []string{"textures/x/a.tga"}) {
		t.Errorf("textures = %v", got)
	}
}

func TestResolveShaderTextures_NamesMatchExactlyFilesIgnoreSlashDirection(t *testing.T) {
	r := mapResolver{
		shaders: map[string]ShaderEntry{
			`models\plant\leaf`:  {Textures: []string{`models\plant\leaf.tga`}, Text: "back"},
			"models/plant/frond": {Textures: []string{"models/plant/frond.tga"}, Text: "fwd"},
		},
		files: map[string]string{
			"models/plant/leaf.tga":  "p",
			"models/plant/frond.tga": "p",
		},
	}
	needed := map[string]bool{}
	used := map[string]ShaderEntry{}
	resolveShaderTextures(`models\plant\leaf`, r, needed, used)
	resolveShaderTextures(`models\plant\frond`, r, needed, used)

	if _, ok := used[`models\plant\leaf`]; !ok {
		t.Error("a backslashed name must match its backslashed definition")
	}
	if len(used) != 1 {
		t.Errorf("a backslashed name must not match a forward-slash definition: used = %v", used)
	}
	for _, f := range []string{"models/plant/leaf.tga", "models/plant/frond.tga"} {
		if !needed[f] {
			t.Errorf("%s not resolved; needed = %v", f, needed)
		}
	}
}

func TestParseBSP_KeepsShaderNamesAsWritten(t *testing.T) {
	bsp := makeTestBSP([]string{`models\plant\leaf`})
	got, err := ParseBSP(strings.NewReader(bsp), int64(len(bsp)))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Shaders) != 1 || got.Shaders[0] != `models\plant\leaf` {
		t.Errorf("shaders = %q", got.Shaders)
	}
}

func TestResolveTexture_EngineExtensionOrder(t *testing.T) {
	index := map[string]string{
		"textures/a/all.png":   "p",
		"textures/a/all.tga":   "p",
		"textures/a/all.jpg":   "p",
		"textures/a/named.jpg": "p",
		"textures/a/named.tga": "p",
	}
	cases := []struct{ ref, want string }{
		{"textures/a/all", "textures/a/all.png"},
		{"textures/a/all.jpg", "textures/a/all.jpg"},
		{"textures/a/named.jpg", "textures/a/named.jpg"},
		{"textures/a/named.png", "textures/a/named.tga"},
	}
	for _, c := range cases {
		if got, _ := ResolveTexture(c.ref, index); got != c.want {
			t.Errorf("ResolveTexture(%q) = %q, want %q", c.ref, got, c.want)
		}
	}
}
