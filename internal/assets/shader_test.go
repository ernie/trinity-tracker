package assets

import (
	"strings"
	"testing"
)

func parseSingleShader(t *testing.T, script string) ShaderTextureDef {
	t.Helper()
	defs, err := ParseShaderTextures(strings.NewReader(script))
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 1 {
		t.Fatalf("expected 1 shader def, got %d", len(defs))
	}
	return defs[0]
}

func TestParseShaderScript_VideoMap(t *testing.T) {
	def := parseSingleShader(t, `
textures/proto2/movie
{
	{
		videoMap video/intro.roq
		blendFunc add
	}
}
`)
	if len(def.Textures) != 1 || def.Textures[0] != "video/intro.roq" {
		t.Errorf("expected [video/intro.roq], got %v", def.Textures)
	}
}

func TestParseShaderScript_VideoMapBareName(t *testing.T) {
	def := parseSingleShader(t, `
textures/proto2/movie
{
	{
		videoMap intro.roq
	}
}
`)
	if len(def.Textures) != 1 || def.Textures[0] != "video/intro.roq" {
		t.Errorf("expected [video/intro.roq], got %v", def.Textures)
	}
}
