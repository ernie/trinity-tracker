package assets

import (
	"os"
	"path/filepath"
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
	if _, ok := bq3["textures/base_wall/stock.tga"]; !ok {
		t.Error("baseq3 file missing from baseq3-only baseline")
	}
	if _, ok := bq3["textures/ta/only.tga"]; ok {
		t.Error("missionpack file must NOT be in baseq3-only baseline")
	}

	both, err := stockBaseline(q3, []string{"baseq3", "missionpack"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := both["textures/ta/only.tga"]; !ok {
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

func TestSplitMapPack_SubtractsStockKeepsCustomAndAAS(t *testing.T) {
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
		"textures/base_wall/retex.tga",
		"textures/ql/custom.tga",
	} {
		if !files[want] {
			t.Errorf("expected %s in map pk3", want)
		}
	}
	for _, notWant := range []string{
		"textures/base_wall/stockwall.tga",
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
	if report.Maps[0].FileCount != 4 {
		t.Errorf("expected 4 kept files (bsp, aas, retex, custom), got %d", report.Maps[0].FileCount)
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
