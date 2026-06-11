package assets

import "testing"

func TestResolveTexture_LiteralNonImagePath(t *testing.T) {
	index := map[string]string{"video/intro.roq": "/tmp/map.pk3"}
	resolved, ok := ResolveTexture("video/intro.roq", index)
	if !ok || resolved != "video/intro.roq" {
		t.Errorf("expected literal video/intro.roq resolved, got %q ok=%v", resolved, ok)
	}
}
