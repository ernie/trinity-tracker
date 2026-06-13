package assets

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestEncodeHeadGeometry(t *testing.T) {
	geo := &MD3Geometry{
		Mins: [3]float32{-1, -2, -3}, Maxs: [3]float32{1, 2, 3},
		Surfaces: []MD3Surface{
			{Name: "h_head", Vertices: []MD3Vertex{
				{Pos: [3]float32{0, 0, 0}, Normal: [3]float32{0, 0, 1}, UV: [2]float32{0, 0}},
				{Pos: [3]float32{1, 0, 0}, Normal: [3]float32{0, 0, 1}, UV: [2]float32{1, 0}},
				{Pos: [3]float32{0, 1, 0}, Normal: [3]float32{0, 0, 1}, UV: [2]float32{0, 1}},
			}, Indices: []uint32{0, 1, 2}},
			{Name: "h_cigar", Vertices: []MD3Vertex{
				{Pos: [3]float32{0, 0, 1}, Normal: [3]float32{0, 0, 1}, UV: [2]float32{0, 0}},
				{Pos: [3]float32{1, 0, 1}, Normal: [3]float32{0, 0, 1}, UV: [2]float32{1, 0}},
				{Pos: [3]float32{0, 1, 1}, Normal: [3]float32{0, 0, 1}, UV: [2]float32{0, 1}},
			}, Indices: []uint32{0, 1, 2}},
		},
	}
	geoBytes, surfaces, vertCount, idxCount := EncodeHeadGeometry(geo)
	if vertCount != 6 || idxCount != 6 {
		t.Fatalf("counts = %d verts / %d idx, want 6/6", vertCount, idxCount)
	}
	if len(surfaces) != 2 {
		t.Fatalf("surfaces = %d, want 2", len(surfaces))
	}
	if surfaces[1].VertOffset != 3 || surfaces[1].IdxOffset != 3 {
		t.Fatalf("surface[1] offsets = %d/%d, want 3/3", surfaces[1].VertOffset, surfaces[1].IdxOffset)
	}
	wantBytes := vertCount*8*4 + idxCount*2
	if len(geoBytes) != wantBytes {
		t.Fatalf("geoBytes = %d, want %d", len(geoBytes), wantBytes)
	}
	idxSectionStart := vertCount * 8 * 4
	got := binary.LittleEndian.Uint16(geoBytes[idxSectionStart+3*2:])
	if got != 3 {
		t.Fatalf("global index[3] = %d, want 3", got)
	}
	if px := math.Float32frombits(binary.LittleEndian.Uint32(geoBytes[0:4])); px != 0 {
		t.Fatalf("vertex[0].px = %v, want 0", px)
	}
}
