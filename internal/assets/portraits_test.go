package assets

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuildPortraitManifest(t *testing.T) {
	dir := t.TempDir()
	mk := func(model string, icons ...string) {
		t.Helper()
		modelDir := filepath.Join(dir, model)
		if err := os.MkdirAll(modelDir, 0755); err != nil {
			t.Fatal(err)
		}
		for _, icon := range icons {
			if err := os.WriteFile(filepath.Join(modelDir, icon), []byte("png"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	mk("sarge", "icon_krusade.png", "icon_default.png", "icon_blue.png")
	mk("brandon", "icon_default.png")
	mk("xaero", "icon_default.png", "notes.txt")
	mk("pi", "icon_default.png")
	mk("empty")
	if os.Geteuid() != 0 {
		mk("locked", "icon_default.png")
		if err := os.Chmod(filepath.Join(dir, "locked"), 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "locked"), 0o755) })
	}

	got, err := BuildPortraitManifest(dir, map[string]bool{"xaero": true, "pi": true})
	if err != nil {
		t.Fatal(err)
	}
	want := []PortraitModel{
		{Model: "brandon", Skins: []string{"default"}},
		{Model: "sarge", Skins: []string{"blue", "default", "krusade"}},
		{Model: "pi", Skins: []string{"default"}},
		{Model: "xaero", Skins: []string{"default"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("manifest = %v, want %v", got, want)
	}
}
