package assets

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHeadManifestStages(t *testing.T) {
	m := HeadManifest{
		Skins: map[string]map[string][]StageManifest{
			"default": {
				"h_visor": {
					{Maps: []string{"snow.png"}, Blend: "opaque", RgbGen: "identity",
						TcMod: []TcMod{{Type: "scroll", Args: []float32{9, 0.3}}}},
					{Maps: []string{"tinfx2b.png"}, Blend: "add", RgbGen: "lightingDiffuse", TcGen: "environment"},
				},
			},
		},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"tcGen":"environment"`) || !strings.Contains(string(b), `"blend":"add"`) {
		t.Fatalf("manifest json missing stage fields: %s", b)
	}
}
