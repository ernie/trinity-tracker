package assets

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PortraitModel is one entry of portraits.json; slice order is the
// picker's presentation order.
type PortraitModel struct {
	Model string   `json:"model"`
	Skins []string `json:"skins"`
}

// tournamentOpponents sit between the baseq3 and Team Arena groups in the
// picker: they read as solo tournament bosses rather than team-centric
// roster, whichever tree their icons ship in.
var tournamentOpponents = []string{"fritzkrieg", "pi"}

// BuildPortraitManifest scans an extracted portraits directory and returns
// one entry per model: baseq3 models (sorted), then tournamentOpponents,
// then Team-Arena-only ones (sorted). taOnly marks models whose default
// icon came from missionpack pk3s; models absent from it group with baseq3.
func BuildPortraitManifest(portraitsDir string, taOnly map[string]bool) ([]PortraitModel, error) {
	entries, err := os.ReadDir(portraitsDir)
	if err != nil {
		return nil, err
	}
	skinsByModel := make(map[string][]string)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(portraitsDir, e.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping portrait model dir %s: %v\n", e.Name(), err)
			continue
		}
		var skins []string
		for _, f := range files {
			name := f.Name()
			if !strings.HasPrefix(name, "icon_") || !strings.HasSuffix(name, ".png") {
				continue
			}
			skins = append(skins, strings.TrimSuffix(strings.TrimPrefix(name, "icon_"), ".png"))
		}
		if len(skins) == 0 {
			continue
		}
		sort.Strings(skins)
		skinsByModel[e.Name()] = skins
	}
	mid := make(map[string]bool, len(tournamentOpponents))
	for _, name := range tournamentOpponents {
		mid[name] = true
	}
	var baseq3, ta []string
	for name := range skinsByModel {
		switch {
		case mid[name]:
		case taOnly[name]:
			ta = append(ta, name)
		default:
			baseq3 = append(baseq3, name)
		}
	}
	sort.Strings(baseq3)
	sort.Strings(ta)
	ordered := append(baseq3, tournamentOpponents...)
	ordered = append(ordered, ta...)
	manifest := make([]PortraitModel, 0, len(skinsByModel))
	for _, name := range ordered {
		if skins, ok := skinsByModel[name]; ok {
			manifest = append(manifest, PortraitModel{Model: name, Skins: skins})
		}
	}
	return manifest, nil
}
