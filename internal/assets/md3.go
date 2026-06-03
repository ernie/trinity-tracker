package assets

import (
	"encoding/binary"
	"fmt"
	"io"
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

	numSurfaces := int32(binary.LittleEndian.Uint32(header[76:80]))
	ofsSurfaces := int64(binary.LittleEndian.Uint32(header[96:100]))

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

		numShaders := int32(binary.LittleEndian.Uint32(header[76:80]))
		// Re-read from surface header
		numShaders = int32(binary.LittleEndian.Uint32(surfHeader[72:76]))
		ofsShaders := int64(binary.LittleEndian.Uint32(surfHeader[88:92]))
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
