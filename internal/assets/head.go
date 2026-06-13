package assets

import (
	"bufio"
	"encoding/binary"
	"io"
	"math"
	"strconv"
	"strings"
)

// HeadSurfaceManifest locates one surface within the encoded geometry.
// Offsets/counts are in element units (vertices, indices).
type HeadSurfaceManifest struct {
	Name       string `json:"name"`
	VertOffset int    `json:"vertOffset"`
	VertCount  int    `json:"vertCount"`
	IdxOffset  int    `json:"idxOffset"`
	IdxCount   int    `json:"idxCount"`
}

// HeadBounds is the model's frame-0 bounding box.
type HeadBounds struct {
	Mins [3]float32 `json:"mins"`
	Maxs [3]float32 `json:"maxs"`
}

// StageManifest is one render pass of a surface in the bundle: a ShaderStage
// with texture references resolved to bundle-local PNGs, plus the surface's
// doubleSided flag.
type StageManifest struct {
	Maps        []string `json:"maps"`
	AnimFreq    float32  `json:"animFreq,omitempty"`
	Blend       string   `json:"blend"`
	AlphaFunc   string   `json:"alphaFunc,omitempty"`
	TcGen       string   `json:"tcGen,omitempty"`
	TcMod       []TcMod  `json:"tcMod,omitempty"`
	RgbGen      string   `json:"rgbGen"`
	Clamp       bool     `json:"clamp,omitempty"`
	DoubleSided bool     `json:"doubleSided,omitempty"`
	Deform      string   `json:"deform,omitempty"`
}

// HeadManifest is head.json: how to interpret head.geo and the ordered render
// stages each surface uses per skin (skin name → surface name → stages).
type HeadManifest struct {
	Bounds      HeadBounds                            `json:"bounds"`
	VertexCount int                                   `json:"vertexCount"`
	IndexCount  int                                   `json:"indexCount"`
	Surfaces    []HeadSurfaceManifest                 `json:"surfaces"`
	Skins       map[string]map[string][]StageManifest `json:"skins"`
	// HeadOffset is the model's animation.cfg headoffset (fwd, y, up): the
	// artist's per-model correction for the HUD head icon framing.
	HeadOffset [3]float32 `json:"headOffset,omitempty"`
}

// ParseHeadOffset reads a model's animation.cfg and returns its headoffset
// (forward, y, up); zero if absent.
func ParseHeadOffset(r io.Reader) [3]float32 {
	var off [3]float32
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) >= 4 && strings.EqualFold(f[0], "headoffset") {
			for i := 0; i < 3; i++ {
				v, _ := strconv.ParseFloat(f[i+1], 32)
				off[i] = float32(v)
			}
			break
		}
	}
	return off
}

// EncodeHeadGeometry serializes frame-0 geometry to the head.geo byte layout:
// interleaved Float32 vertices, then global Uint16 indices.
func EncodeHeadGeometry(geo *MD3Geometry) (geoBytes []byte, surfaces []HeadSurfaceManifest, vertCount, idxCount int) {
	for _, s := range geo.Surfaces {
		vertCount += len(s.Vertices)
		idxCount += len(s.Indices)
	}
	geoBytes = make([]byte, vertCount*8*4+idxCount*2)

	vp := 0
	vertBase := 0
	idxBase := 0
	idxSectionStart := vertCount * 8 * 4
	ip := idxSectionStart
	for _, s := range geo.Surfaces {
		for _, v := range s.Vertices {
			for _, f := range [8]float32{v.Pos[0], v.Pos[1], v.Pos[2], v.Normal[0], v.Normal[1], v.Normal[2], v.UV[0], v.UV[1]} {
				binary.LittleEndian.PutUint32(geoBytes[vp:], math.Float32bits(f))
				vp += 4
			}
		}
		// Global indices let the browser draw every surface from one bound buffer.
		for _, idx := range s.Indices {
			binary.LittleEndian.PutUint16(geoBytes[ip:], uint16(int(idx)+vertBase))
			ip += 2
		}
		surfaces = append(surfaces, HeadSurfaceManifest{
			Name:       s.Name,
			VertOffset: vertBase, VertCount: len(s.Vertices),
			IdxOffset: idxBase, IdxCount: len(s.Indices),
		})
		vertBase += len(s.Vertices)
		idxBase += len(s.Indices)
	}
	return geoBytes, surfaces, vertCount, idxCount
}
