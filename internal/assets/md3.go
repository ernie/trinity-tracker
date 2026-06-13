package assets

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
)

const (
	md3Magic      = "IDP3"
	md3Version    = 15
	md3HeaderSize = 108
	md3ShaderSize = 68 // 64-byte name + int32 index
)

// ParseMD3Shaders parses an MD3 model file and extracts surface shader references.
func ParseMD3Shaders(r io.ReaderAt, size int64) ([]string, error) {
	if size < md3HeaderSize {
		return nil, fmt.Errorf("MD3 too small: %d bytes", size)
	}

	// Read header
	header := make([]byte, md3HeaderSize)
	if _, err := r.ReadAt(header, 0); err != nil {
		return nil, fmt.Errorf("read MD3 header: %w", err)
	}

	// Verify magic and version
	if string(header[0:4]) != md3Magic {
		return nil, fmt.Errorf("invalid MD3 magic: %q", header[0:4])
	}
	version := int32(binary.LittleEndian.Uint32(header[4:8]))
	if version != md3Version {
		return nil, fmt.Errorf("unsupported MD3 version: %d", version)
	}

	// md3Header_t field offsets (qfiles.h): ident 0, version 4, name 8,
	// flags 72, numFrames 76, numTags 80, numSurfaces 84, numSkins 88,
	// ofsFrames 92, ofsTags 96, ofsSurfaces 100, ofsEnd 104
	numSurfaces := int32(binary.LittleEndian.Uint32(header[84:88]))
	ofsSurfaces := int64(binary.LittleEndian.Uint32(header[100:104]))

	var shaders []string
	seen := make(map[string]bool)

	surfaceOfs := ofsSurfaces
	for i := int32(0); i < numSurfaces; i++ {
		if surfaceOfs+12*4+64+4 > size {
			break
		}

		// Read surface header (enough to get shader info)
		surfHeader := make([]byte, 12*4+64+4) // up through ofsEnd fields
		if _, err := r.ReadAt(surfHeader, surfaceOfs); err != nil {
			return nil, fmt.Errorf("read MD3 surface %d header: %w", i, err)
		}

		// Verify surface magic
		if string(surfHeader[0:4]) != md3Magic {
			return nil, fmt.Errorf("invalid MD3 surface magic at offset %d", surfaceOfs)
		}

		// md3Surface_t field offsets (qfiles.h): ident 0, name 4, flags 68,
		// numFrames 72, numShaders 76, numVerts 80, numTriangles 84,
		// ofsTriangles 88, ofsShaders 92, ofsSt 96, ofsXyzNormals 100, ofsEnd 104
		numShaders := int32(binary.LittleEndian.Uint32(surfHeader[76:80]))
		ofsShaders := int64(binary.LittleEndian.Uint32(surfHeader[92:96]))
		ofsEnd := int64(binary.LittleEndian.Uint32(surfHeader[104:108]))

		// Read shader entries
		for j := int32(0); j < numShaders; j++ {
			shaderOfs := surfaceOfs + ofsShaders + int64(j)*md3ShaderSize
			if shaderOfs+md3ShaderSize > size {
				break
			}
			shaderData := make([]byte, md3ShaderSize)
			if _, err := r.ReadAt(shaderData, shaderOfs); err != nil {
				break
			}
			name := strings.ReplaceAll(readNullTerminated(shaderData[:64]), "\\", "/")
			if name != "" && !seen[name] {
				seen[name] = true
				shaders = append(shaders, name)
			}
		}

		surfaceOfs += ofsEnd
	}

	return shaders, nil
}

type MD3Vertex struct {
	Pos    [3]float32
	Normal [3]float32
	UV     [2]float32
}

type MD3Surface struct {
	Name     string
	Vertices []MD3Vertex
	Indices  []uint32
}

type MD3Geometry struct {
	Surfaces   []MD3Surface
	Mins, Maxs [3]float32
}

// ParseMD3Geometry parses an MD3 model file and extracts frame-0 geometry.
func ParseMD3Geometry(r io.ReaderAt, size int64) (*MD3Geometry, error) {
	if size < md3HeaderSize {
		return nil, fmt.Errorf("MD3 too small: %d bytes", size)
	}

	header := make([]byte, md3HeaderSize)
	if _, err := r.ReadAt(header, 0); err != nil {
		return nil, fmt.Errorf("read MD3 header: %w", err)
	}
	if string(header[0:4]) != md3Magic {
		return nil, fmt.Errorf("invalid MD3 magic: %q", header[0:4])
	}
	version := int32(binary.LittleEndian.Uint32(header[4:8]))
	if version != md3Version {
		return nil, fmt.Errorf("unsupported MD3 version: %d", version)
	}

	numSurfaces := int32(binary.LittleEndian.Uint32(header[84:88]))
	ofsSurfaces := int64(binary.LittleEndian.Uint32(header[100:104]))

	geo := &MD3Geometry{
		Mins: [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32},
		Maxs: [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32},
	}

	surfaceOfs := ofsSurfaces
	for i := int32(0); i < numSurfaces; i++ {
		if surfaceOfs+md3HeaderSize > size {
			break
		}

		surfHeader := make([]byte, md3HeaderSize)
		if _, err := r.ReadAt(surfHeader, surfaceOfs); err != nil {
			return nil, fmt.Errorf("read MD3 surface %d header: %w", i, err)
		}
		if string(surfHeader[0:4]) != md3Magic {
			return nil, fmt.Errorf("invalid MD3 surface magic at offset %d", surfaceOfs)
		}

		name := strings.ReplaceAll(readNullTerminated(surfHeader[4:68]), "\\", "/")
		numVerts := int32(binary.LittleEndian.Uint32(surfHeader[80:84]))
		numTriangles := int32(binary.LittleEndian.Uint32(surfHeader[84:88]))
		ofsTriangles := int64(binary.LittleEndian.Uint32(surfHeader[88:92]))
		ofsSt := int64(binary.LittleEndian.Uint32(surfHeader[96:100]))
		ofsXyzNormals := int64(binary.LittleEndian.Uint32(surfHeader[100:104]))
		ofsEnd := int64(binary.LittleEndian.Uint32(surfHeader[104:108]))

		st := make([]byte, int64(numVerts)*8)
		if _, err := r.ReadAt(st, surfaceOfs+ofsSt); err != nil {
			return nil, fmt.Errorf("read MD3 surface %d texcoords: %w", i, err)
		}
		xyz := make([]byte, int64(numVerts)*8)
		if _, err := r.ReadAt(xyz, surfaceOfs+ofsXyzNormals); err != nil {
			return nil, fmt.Errorf("read MD3 surface %d xyz: %w", i, err)
		}

		verts := make([]MD3Vertex, numVerts)
		for v := int32(0); v < numVerts; v++ {
			var vert MD3Vertex
			vert.UV[0] = math.Float32frombits(binary.LittleEndian.Uint32(st[v*8 : v*8+4]))
			vert.UV[1] = math.Float32frombits(binary.LittleEndian.Uint32(st[v*8+4 : v*8+8]))

			const scale = 1.0 / 64.0
			vert.Pos[0] = float32(int16(binary.LittleEndian.Uint16(xyz[v*8:v*8+2]))) * scale
			vert.Pos[1] = float32(int16(binary.LittleEndian.Uint16(xyz[v*8+2:v*8+4]))) * scale
			vert.Pos[2] = float32(int16(binary.LittleEndian.Uint16(xyz[v*8+4:v*8+6]))) * scale

			enc := binary.LittleEndian.Uint16(xyz[v*8+6 : v*8+8])
			// Engine quantizes the normal's lat/lng over 256 steps (tr_surface.c).
			lat := float64((enc>>8)&0xff) * (2 * math.Pi / 256)
			lng := float64(enc&0xff) * (2 * math.Pi / 256)
			vert.Normal[0] = float32(math.Cos(lat) * math.Sin(lng))
			vert.Normal[1] = float32(math.Sin(lat) * math.Sin(lng))
			vert.Normal[2] = float32(math.Cos(lng))

			for c := 0; c < 3; c++ {
				if vert.Pos[c] < geo.Mins[c] {
					geo.Mins[c] = vert.Pos[c]
				}
				if vert.Pos[c] > geo.Maxs[c] {
					geo.Maxs[c] = vert.Pos[c]
				}
			}
			verts[v] = vert
		}

		tris := make([]byte, int64(numTriangles)*12)
		if _, err := r.ReadAt(tris, surfaceOfs+ofsTriangles); err != nil {
			return nil, fmt.Errorf("read MD3 surface %d triangles: %w", i, err)
		}
		indices := make([]uint32, 0, numTriangles*3)
		for t := int32(0); t < numTriangles*3; t++ {
			indices = append(indices, binary.LittleEndian.Uint32(tris[t*4:t*4+4]))
		}

		geo.Surfaces = append(geo.Surfaces, MD3Surface{
			Name:     name,
			Vertices: verts,
			Indices:  indices,
		})

		surfaceOfs += ofsEnd
	}

	return geo, nil
}
