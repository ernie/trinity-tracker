package assets

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

// buildTestMD3 constructs a minimal valid MD3 with one surface and the
// given shader names, following md3Header_t/md3Surface_t in the engine's
// qfiles.h. A nonzero numTags ensures ofsTags != ofsSurfaces, so offset
// regressions can't pass by coincidence (the original parser bug did).
func buildTestMD3(shaderNames []string) []byte {
	var buf bytes.Buffer
	w := func(v uint32) { binary.Write(&buf, binary.LittleEndian, v) }

	const headerSize = 108
	const tagSize = 64 + 12*4 // name[64] + origin + 3 axis vectors
	const surfHeaderSize = 108
	numTags := uint32(1)
	ofsTags := uint32(headerSize)
	ofsSurfaces := ofsTags + numTags*tagSize
	surfShadersLen := uint32(len(shaderNames)) * md3ShaderSize
	surfEnd := uint32(surfHeaderSize) + surfShadersLen
	ofsEnd := ofsSurfaces + surfEnd

	// md3Header_t
	buf.WriteString(md3Magic)
	w(md3Version)
	buf.Write(make([]byte, 64)) // name
	w(0)                        // flags
	w(1)                        // numFrames
	w(numTags)
	w(1) // numSurfaces
	w(0) // numSkins
	w(ofsTags)
	w(ofsTags)
	w(ofsSurfaces)
	w(ofsEnd)

	buf.Write(make([]byte, numTags*tagSize))

	// md3Surface_t
	buf.WriteString(md3Magic)
	buf.Write(make([]byte, 64))        // name
	w(0)                               // flags
	w(1)                               // numFrames
	w(uint32(len(shaderNames)))        // numShaders
	w(0)                               // numVerts
	w(0)                               // numTriangles
	w(surfHeaderSize + surfShadersLen) // ofsTriangles
	w(surfHeaderSize)                  // ofsShaders
	w(surfHeaderSize + surfShadersLen) // ofsSt
	w(surfHeaderSize + surfShadersLen) // ofsXyzNormals
	w(surfEnd)                         // ofsEnd

	// md3Shader_t entries
	for _, name := range shaderNames {
		entry := make([]byte, md3ShaderSize)
		copy(entry, name)
		buf.Write(entry)
	}

	return buf.Bytes()
}

func TestParseMD3Shaders(t *testing.T) {
	want := []string{"textures/13castle/flag_0.tga", "models/mapobjects/lamp/lamp"}
	data := buildTestMD3(want)

	got, err := ParseMD3Shaders(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("ParseMD3Shaders: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d shader refs %q, want %d %q", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("shader[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveShaderTexturesStripsExtension(t *testing.T) {
	// MD3 shader refs often carry an extension ("flag_0.tga") while
	// shader scripts define names without one; the engine strips before
	// lookup (R_FindShader → COM_StripExtension), and the resolver must too.
	gm := &GameManifest{
		Shaders: map[string][]string{
			"textures/13castle/flag_0": {"textures/13castle/xflag01.tga"},
		},
		ShaderFiles: map[string]string{
			"textures/13castle/flag_0": "scripts/13castle.shader",
		},
		FileIndex: map[string]string{
			"textures/13castle/xflag01.tga": "map-13castle.pk3",
			"scripts/13castle.shader":       "map-13castle.pk3",
		},
	}

	needed := make(map[string]bool)
	resolveShaderTextures("textures/13castle/flag_0.tga", gm, needed)

	if !needed["textures/13castle/xflag01.tga"] {
		t.Errorf("xflag01.tga not resolved from extension-bearing shader ref; needed = %v", needed)
	}
	if !needed["scripts/13castle.shader"] {
		t.Errorf("shader script not included; needed = %v", needed)
	}
}

func TestParseMD3Geometry(t *testing.T) {
	data, err := os.ReadFile("testdata/sarge_head.md3")
	if err != nil {
		t.Skipf("no fixture: %v", err)
	}
	geo, err := ParseMD3Geometry(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(geo.Surfaces) == 0 {
		t.Fatal("expected at least one surface")
	}
	for _, s := range geo.Surfaces {
		if len(s.Vertices) == 0 || len(s.Indices) == 0 {
			t.Fatalf("surface %q has no geometry", s.Name)
		}
		if len(s.Indices)%3 != 0 {
			t.Fatalf("surface %q index count %d not a multiple of 3", s.Name, len(s.Indices))
		}
		for _, idx := range s.Indices {
			if int(idx) >= len(s.Vertices) {
				t.Fatalf("surface %q index %d out of range (%d verts)", s.Name, idx, len(s.Vertices))
			}
		}
		for _, v := range s.Vertices {
			if v.UV[0] < -2 || v.UV[0] > 2 || v.UV[1] < -2 || v.UV[1] > 2 {
				t.Fatalf("surface %q UV out of sane range: %v", s.Name, v.UV)
			}
		}
	}
	if geo.Maxs[2] <= geo.Mins[2] {
		t.Fatalf("degenerate bounds: mins=%v maxs=%v", geo.Mins, geo.Maxs)
	}
}
