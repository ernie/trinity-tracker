package assets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStockBaseline_ScopesByGameAndErrorsWhenEmpty(t *testing.T) {
	q3 := t.TempDir()

	if _, err := stockBaseline(q3, []string{"baseq3"}); err == nil {
		t.Fatal("expected error when no official paks present")
	}

	baseq3 := filepath.Join(q3, "baseq3")
	mp := filepath.Join(q3, "missionpack")
	for _, d := range []string{baseq3, mp} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeTestPk3(t, baseq3, "pak0.pk3", map[string]string{"textures/base_wall/stock.tga": "B"})
	writeTestPk3(t, mp, "pak0.pk3", map[string]string{"textures/ta/only.tga": "M"})

	bq3, err := stockBaseline(q3, []string{"baseq3"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := bq3.files["textures/base_wall/stock.tga"]; !ok {
		t.Error("baseq3 file missing from baseq3-only baseline")
	}
	if _, ok := bq3.files["textures/ta/only.tga"]; ok {
		t.Error("missionpack file must NOT be in baseq3-only baseline")
	}

	both, err := stockBaseline(q3, []string{"baseq3", "missionpack"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := both.files["textures/ta/only.tga"]; !ok {
		t.Error("missionpack file should be present when missionpack is in the baseline")
	}
}

func TestScanSources_LaterSourceWins(t *testing.T) {
	dir := t.TempDir()
	a := writeTestPk3(t, dir, "a.pk3", map[string]string{"shared.txt": "AAA"})
	b := writeTestPk3(t, dir, "b.pk3", map[string]string{"shared.txt": "BBBB", "only_b.txt": "B"})

	idx, err := scanSources([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if idx["shared.txt"].pk3 != b {
		t.Errorf("later source should win: got %s", idx["shared.txt"].pk3)
	}
	if idx["only_b.txt"].pk3 != b {
		t.Error("only_b.txt missing")
	}
}

func TestSplitMapPack_DropsStockPathsKeepsCustomAndAAS(t *testing.T) {
	root := t.TempDir()

	baseq3 := filepath.Join(root, "q3", "baseq3")
	if err := os.MkdirAll(baseq3, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestPk3(t, baseq3, "pak0.pk3", map[string]string{
		"textures/base_wall/stockwall.tga": "STOCK",
		"textures/base_wall/retex.tga":     "ORIGINAL",
	})

	srcDir := filepath.Join(root, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	source := writeTestPk3(t, srcDir, "pack.pk3", map[string]string{
		"maps/arena.bsp": makeTestBSP([]string{
			"textures/base_wall/stockwall",
			"textures/base_wall/retex",
			"textures/ql/custom",
		}),
		"maps/arena.aas":                   "AASDATA",
		"textures/base_wall/stockwall.tga": "STOCK",
		"textures/base_wall/retex.tga":     "QLVERSION",
		"textures/ql/custom.tga":           "CUSTOM",
		"textures/unused/junk.tga":         "JUNK",
	})

	out := filepath.Join(root, "out")
	report, err := SplitMapPack(SplitOptions{
		Sources:   []string{source},
		Quake3Dir: filepath.Join(root, "q3"),
		OutputDir: out,
	})
	if err != nil {
		t.Fatal(err)
	}

	files, err := MapPakFileSet(filepath.Join(out, "arena.pk3"))
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"maps/arena.bsp",
		"maps/arena.aas",
		"textures/ql/custom.tga",
	} {
		if !files[want] {
			t.Errorf("expected %s in map pk3", want)
		}
	}
	for _, notWant := range []string{
		"textures/base_wall/stockwall.tga",
		"textures/base_wall/retex.tga",
		"textures/unused/junk.tga",
	} {
		if files[notWant] {
			t.Errorf("did not expect %s in map pk3", notWant)
		}
	}

	if report.DeadFiles != 1 {
		t.Errorf("expected 1 dead file (junk), got %d", report.DeadFiles)
	}
	if len(report.Maps) != 1 || report.Maps[0].Name != "arena" {
		t.Errorf("expected one map report for arena, got %+v", report.Maps)
	}
	if report.Maps[0].FileCount != 3 {
		t.Errorf("expected 3 kept files (bsp, aas, custom), got %d", report.Maps[0].FileCount)
	}
}

// splitFixture lays out a Quake III dir with the given stock paks and a
// source pack, runs the split, and returns the output dir.
func splitFixture(t *testing.T, stock map[string][][2]string, source ...[2]string) string {
	t.Helper()
	root := t.TempDir()
	baseq3 := filepath.Join(root, "q3", "baseq3")
	if err := os.MkdirAll(baseq3, 0755); err != nil {
		t.Fatal(err)
	}
	for name, entries := range stock {
		writeOrderedTestPk3(t, baseq3, name, entries...)
	}
	src := writeOrderedTestPk3(t, root, "pack.pk3", source...)
	out := filepath.Join(root, "out")
	if _, err := SplitMapPack(SplitOptions{
		Sources:   []string{src},
		Quake3Dir: filepath.Join(root, "q3"),
		OutputDir: out,
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func readSplitFile(t *testing.T, out, mapName, path string) (string, bool) {
	t.Helper()
	files, err := MapPakFileSet(filepath.Join(out, mapName+".pk3"))
	if err != nil {
		t.Fatal(err)
	}
	if !files[path] {
		return "", false
	}
	data, err := ReadFileFromPk3(filepath.Join(out, mapName+".pk3"), path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), true
}

func TestSplitMapPack_DropsTrinityPakPaths(t *testing.T) {
	out := splitFixture(t,
		map[string][][2]string{
			"pak0.pk3":  {{"gfx/2d/bigchars.tga", "B"}},
			"pak8t.pk3": {{"textures/sfx/pent.jpg", "HD"}},
		},
		[2]string{"maps/arena.bsp", makeTestBSP([]string{"textures/sfx/pent", "textures/ql/custom"})},
		[2]string{"textures/sfx/pent.jpg", "OLD"},
		[2]string{"textures/ql/custom.tga", "C"},
	)
	if _, ok := readSplitFile(t, out, "arena", "textures/sfx/pent.jpg"); ok {
		t.Error("a path pak8t ships must be dropped")
	}
	if _, ok := readSplitFile(t, out, "arena", "textures/ql/custom.tga"); !ok {
		t.Error("custom texture missing")
	}
}

func TestSplitMapPack_BundlesSourceImagesForStockShaders(t *testing.T) {
	out := splitFixture(t,
		map[string][][2]string{
			"pak0.pk3": {{"scripts/sfx.shader", "textures/sfx/pureblack\n{\n\t{\n\t\tmap textures/skies/blacksky.tga\n\t}\n}\n"}},
		},
		[2]string{"maps/arena.bsp", makeTestBSP([]string{"textures/sfx/pureblack"})},
		[2]string{"textures/skies/blacksky.jpg", "SKY"},
	)
	if _, ok := readSplitFile(t, out, "arena", "textures/skies/blacksky.jpg"); !ok {
		t.Error("image referenced by a stock shader and shipped only by the source must be bundled")
	}
	if text, ok := readSplitFile(t, out, "arena", "scripts/mapsplit_arena.shader"); ok && strings.Contains(text, "pureblack") {
		t.Error("a stock shader must not be redefined in the generated shader file")
	}
}

func TestSplitMapPack_StockDefinitionBeatsSource(t *testing.T) {
	out := splitFixture(t,
		map[string][][2]string{
			"pak0.pk3": {{"scripts/ctf.shader", "textures/ctf/flag\n{\n\t{\n\t\tmap textures/ctf/stockflag.tga\n\t}\n}\n"}},
		},
		[2]string{"maps/arena.bsp", makeTestBSP([]string{"textures/ctf/flag"})},
		[2]string{"scripts/qlctf.shader", "textures/ctf/flag\n{\n\t{\n\t\tmap textures/ctf/qlflag.tga\n\t}\n}\n"},
		[2]string{"textures/ctf/stockflag.tga", "S"},
		[2]string{"textures/ctf/qlflag.tga", "Q"},
	)
	if _, ok := readSplitFile(t, out, "arena", "textures/ctf/stockflag.tga"); !ok {
		t.Error("the stock definition's image must be bundled")
	}
	if _, ok := readSplitFile(t, out, "arena", "textures/ctf/qlflag.tga"); ok {
		t.Error("the source's redefinition must not be used")
	}
	if _, ok := readSplitFile(t, out, "arena", "scripts/qlctf.shader"); ok {
		t.Error("source shader scripts must not be bundled")
	}
}

func TestSplitMapPack_GeneratesConsistentShaderFiles(t *testing.T) {
	first := "textures/ql/floor\n{\n\tpolygonOffset\n\t{\n\t\tmap textures/ql/floor_a.tga\n\t}\n}"
	out := splitFixture(t,
		map[string][][2]string{"pak0.pk3": {{"gfx/2d/bigchars.tga", "B"}}},
		[2]string{"maps/alpha.bsp", makeTestBSP([]string{"textures/ql/floor"})},
		[2]string{"maps/zulu.bsp", makeTestBSP([]string{"textures/ql/floor", "textures/ql/zulu_only"})},
		[2]string{"scripts/alpha.shader", first + "\n"},
		[2]string{"scripts/zulu.shader", "textures/ql/floor\n{\n\t{\n\t\tmap textures/ql/floor_b.tga\n\t}\n}\ntextures/ql/zulu_only\n{\n\t{\n\t\tmap textures/ql/z.tga\n\t}\n}\n"},
		[2]string{"textures/ql/floor_a.tga", "A"},
		[2]string{"textures/ql/floor_b.tga", "B"},
		[2]string{"textures/ql/z.tga", "Z"},
	)
	for _, m := range []string{"alpha", "zulu"} {
		text, ok := readSplitFile(t, out, m, "scripts/mapsplit_"+m+".shader")
		if !ok {
			t.Fatalf("%s: generated shader file missing", m)
		}
		if !strings.Contains(text, first) {
			t.Errorf("%s: generated file lacks the pack's winning definition:\n%s", m, text)
		}
		if strings.Contains(text, "floor_b") {
			t.Errorf("%s: generated file carries the losing definition", m)
		}
		for _, script := range []string{"scripts/alpha.shader", "scripts/zulu.shader"} {
			if _, ok := readSplitFile(t, out, m, script); ok {
				t.Errorf("%s: source script %s must not be bundled", m, script)
			}
		}
	}
	if text, _ := readSplitFile(t, out, "alpha", "scripts/mapsplit_alpha.shader"); strings.Contains(text, "zulu_only") {
		t.Error("alpha's generated file must hold only the shaders alpha draws")
	}
	if _, ok := readSplitFile(t, out, "zulu", "textures/ql/z.tga"); !ok {
		t.Error("zulu_only's image missing")
	}
}

func TestSplitMapPack_WritesPerMapArenaFromCombinedFiles(t *testing.T) {
	out := splitFixture(t,
		map[string][][2]string{"pak0.pk3": {{"gfx/2d/bigchars.tga", "B"}}},
		[2]string{"maps/alpha.bsp", makeTestBSP([]string{"textures/ql/a"})},
		[2]string{"maps/zulu.bsp", makeTestBSP([]string{"textures/ql/a"})},
		[2]string{"textures/ql/a.tga", "A"},
		[2]string{"scripts/pack_01.arena", "{\nmap \"alpha\"\nlongname \"Alpha\"\ntype \"ffa\"\n}\n\n{\nmap \"zulu\"\nlongname \"Zulu\"\ntype \"ctf\"\n}\n"},
	)
	text, ok := readSplitFile(t, out, "alpha", "scripts/alpha.arena")
	if !ok {
		t.Fatal("alpha.arena missing")
	}
	if !strings.Contains(text, `longname "Alpha"`) || strings.Contains(text, "Zulu") {
		t.Errorf("alpha.arena should hold only alpha's entry:\n%s", text)
	}
	if text, ok := readSplitFile(t, out, "zulu", "scripts/zulu.arena"); !ok || !strings.Contains(text, `type "ctf"`) {
		t.Errorf("zulu.arena wrong or missing:\n%s", text)
	}
	if _, ok := readSplitFile(t, out, "alpha", "scripts/pack_01.arena"); ok {
		t.Error("the combined arena file must not be bundled")
	}
}

func TestSplitMapPack_ErrorsWithoutStock(t *testing.T) {
	root := t.TempDir()
	source := writeTestPk3(t, root, "pack.pk3", map[string]string{
		"maps/arena.bsp": makeTestBSP([]string{"textures/ql/custom"}),
	})
	_, err := SplitMapPack(SplitOptions{
		Sources:   []string{source},
		Quake3Dir: filepath.Join(root, "no-such-q3"),
		OutputDir: filepath.Join(root, "out"),
	})
	if err == nil {
		t.Fatal("expected error when stock baseline is missing")
	}
}

func TestSplitMapPack_DefaultBaselineKeepsMissionpackPathAsset(t *testing.T) {
	root := t.TempDir()
	baseq3 := filepath.Join(root, "q3", "baseq3")
	mp := filepath.Join(root, "q3", "missionpack")
	for _, d := range []string{baseq3, mp} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeTestPk3(t, baseq3, "pak0.pk3", map[string]string{"textures/base_wall/stock.tga": "B"})
	writeTestPk3(t, mp, "pak0.pk3", map[string]string{"textures/ta/shared.tga": "TA"})

	srcDir := filepath.Join(root, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	source := writeTestPk3(t, srcDir, "pack.pk3", map[string]string{
		"maps/arena.bsp":         makeTestBSP([]string{"textures/ta/shared"}),
		"textures/ta/shared.tga": "TA",
	})

	out := filepath.Join(root, "out")
	if _, err := SplitMapPack(SplitOptions{
		Sources:   []string{source},
		Quake3Dir: filepath.Join(root, "q3"),
		OutputDir: out,
	}); err != nil {
		t.Fatal(err)
	}
	files, err := MapPakFileSet(filepath.Join(out, "arena.pk3"))
	if err != nil {
		t.Fatal(err)
	}
	if !files["textures/ta/shared.tga"] {
		t.Error("baseq3-only baseline must KEEP a missionpack-path asset (client may lack Team Arena)")
	}

	out2 := filepath.Join(root, "out2")
	if _, err := SplitMapPack(SplitOptions{
		Sources:       []string{source},
		Quake3Dir:     filepath.Join(root, "q3"),
		OutputDir:     out2,
		BaselineGames: []string{"baseq3", "missionpack"},
	}); err != nil {
		t.Fatal(err)
	}
	files2, err := MapPakFileSet(filepath.Join(out2, "arena.pk3"))
	if err != nil {
		t.Fatal(err)
	}
	if files2["textures/ta/shared.tga"] {
		t.Error("with missionpack in the baseline, the TA-path asset should be dropped")
	}
}

func TestMissingAudio_FlagsOnlyTrulyUnresolvable(t *testing.T) {
	source := map[string]string{
		"sound/custom/ok.wav": "p",
		"sound/bad/x.ogg.wav": "p",
	}
	stock := map[string]string{
		"sound/world/stock.wav": "",
	}
	refs := []string{
		"sound/custom/ok.wav",
		"sound/world/stock.ogg",
		"sound/bad/x.ogg",
		"music/missing.wav",
		"music/missing.wav",
	}
	got := missingAudio(refs, source, stock)
	want := map[string]bool{"music/missing.wav": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, g := range got {
		if !want[g] {
			t.Errorf("unexpected flagged ref %q", g)
		}
	}
}

func TestResolveAudioPath_AppendsToFullName(t *testing.T) {
	idx := map[string]string{
		"sound/a/exact.wav":    "p",
		"sound/b/stock.wav":    "p",
		"sound/c/loop.ogg.wav": "p",
	}
	cases := []struct {
		ref, want string
		ok        bool
	}{
		{"sound/a/exact.wav", "sound/a/exact.wav", true},
		{"sound/b/stock.ogg", "sound/b/stock.wav", true},
		{"sound/c/loop.ogg", "sound/c/loop.ogg.wav", true},
		{"sound/d/missing.ogg", "", false},
	}
	for _, c := range cases {
		got, ok := resolveAudioPath(c.ref, idx)
		if got != c.want || ok != c.ok {
			t.Errorf("resolveAudioPath(%q) = (%q, %v); want (%q, %v)", c.ref, got, ok, c.want, c.ok)
		}
	}
}
