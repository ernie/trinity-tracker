package assets

import (
	"slices"
	"testing"
)

func TestParseEntitiesFuncPlatImpliesPlatSounds(t *testing.T) {
	ents := `{
"classname" "worldspawn"
"message" "Test Map"
}
{
"classname" "func_plat"
"model" "*1"
}
`
	assets := &BSPAssets{}
	parseEntities(ents, assets)

	for _, want := range []string{
		"sound/movers/plats/pt1_strt.wav",
		"sound/movers/plats/pt1_end.wav",
	} {
		if !slices.Contains(assets.Sounds, want) {
			t.Errorf("Sounds missing %s, got %v", want, assets.Sounds)
		}
	}
}

func TestParseEntitiesNoFuncPlatNoPlatSounds(t *testing.T) {
	ents := `{
"classname" "worldspawn"
}
{
"classname" "func_door"
"model" "*1"
}
{
"classname" "target_speaker"
"noise" "sound/world/battle4.wav"
}
`
	assets := &BSPAssets{}
	parseEntities(ents, assets)

	for _, s := range assets.Sounds {
		if s != "sound/world/battle4.wav" {
			t.Errorf("unexpected implied sound %s", s)
		}
	}
}
