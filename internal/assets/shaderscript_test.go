package assets

import (
	"strings"
	"testing"
)

const shaderFixture = `
models/players/hunter/hunter_f
{
     deformVertexes wave 100 sin 0 .3 0 .2
     cull disable
        {
                map models/players/hunter/hunter_f.tga
                alphaFunc GE128
                depthWrite
                rgbGen lightingDiffuse
        }
}
models/players/crash/crash_f
{
        {
                map textures/sfx/snow.tga
                tcmod scale .5 .5
                tcmod scroll  9 0.3
                rgbGen identity
        }
        {
                map textures/effects/tinfx2b.tga
                tcGen environment
                blendFunc add
                rgbGen lightingdiffuse
        }
}
`

func TestParseShaderScript(t *testing.T) {
	defs, err := ParseShaderScript(strings.NewReader(shaderFixture))
	if err != nil {
		t.Fatal(err)
	}
	hf, ok := defs["models/players/hunter/hunter_f"]
	if !ok {
		t.Fatal("missing hunter_f")
	}
	if !hf.DoubleSided {
		t.Error("hunter_f should be double-sided (cull disable)")
	}
	if len(hf.Stages) != 1 {
		t.Fatalf("hunter_f stages = %d, want 1", len(hf.Stages))
	}
	if hf.Stages[0].AlphaFunc != "ge128" {
		t.Errorf("hunter_f alphaFunc = %q, want ge128", hf.Stages[0].AlphaFunc)
	}
	if hf.Stages[0].Blend != "opaque" {
		t.Errorf("hunter_f blend = %q, want opaque", hf.Stages[0].Blend)
	}
	if got := hf.Stages[0].Maps; len(got) != 1 || got[0] != "models/players/hunter/hunter_f.tga" {
		t.Errorf("hunter_f maps = %v", got)
	}
	cf := defs["models/players/crash/crash_f"]
	if len(cf.Stages) != 2 {
		t.Fatalf("crash_f stages = %d, want 2", len(cf.Stages))
	}
	s0 := cf.Stages[0]
	if s0.RgbGen != "identity" || s0.Blend != "opaque" {
		t.Errorf("crash_f stage0 rgbGen=%q blend=%q", s0.RgbGen, s0.Blend)
	}
	if len(s0.TcMod) != 2 || s0.TcMod[0].Type != "scale" || s0.TcMod[1].Type != "scroll" {
		t.Errorf("crash_f stage0 tcMod = %+v", s0.TcMod)
	}
	if s0.TcMod[1].Args[0] != 9 {
		t.Errorf("crash_f scroll u = %v, want 9", s0.TcMod[1].Args[0])
	}
	s1 := cf.Stages[1]
	if s1.TcGen != "environment" || s1.Blend != "add" || s1.RgbGen != "lightingDiffuse" {
		t.Errorf("crash_f stage1 = %+v", s1)
	}
}

func TestMergeShaderDefs(t *testing.T) {
	a, _ := ParseShaderScript(strings.NewReader("foo/bar\n{\n{\nmap a.tga\n}\n}\n"))
	b, _ := ParseShaderScript(strings.NewReader("baz/qux\n{\n{\nmap b.tga\nblendFunc add\n}\n}\n"))
	idx := map[string]ShaderDef{}
	MergeShaderDefs(idx, a)
	MergeShaderDefs(idx, b)
	if _, ok := idx["foo/bar"]; !ok {
		t.Error("missing foo/bar")
	}
	if idx["baz/qux"].Stages[0].Blend != "add" {
		t.Error("baz/qux blend not merged")
	}
}
