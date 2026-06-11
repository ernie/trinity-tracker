package assets

import (
	"strings"
	"testing"
)

func parseSingleShader(t *testing.T, script string) ShaderDef {
	t.Helper()
	defs, err := ParseShaderScript(strings.NewReader(script))
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
	// The engine prepends video/ when the name has no path
	// (cl_cin.c CIN_PlayCinematic) — the parser must mirror that.
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
