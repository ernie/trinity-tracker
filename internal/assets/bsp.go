package assets

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

const (
	bspMagic        = "IBSP"
	bspVersion      = 0x2E
	bspLumpEntities = 0
	bspLumpShaders  = 1
	bspNumLumps     = 17
	bspShaderSize   = 72                // 64 bytes name + 2x int32
	bspHeaderSize   = 8 + bspNumLumps*8 // magic(4) + version(4) + 17 lumps * (offset(4) + length(4))
)

// BSPAssets holds asset references extracted from a BSP file.
type BSPAssets struct {
	Shaders []string
	Music   []string
	Sounds  []string
	Models  []string
}

// ParseBSP parses a Q3 BSP file and extracts asset references.
func ParseBSP(r io.ReaderAt, size int64) (*BSPAssets, error) {
	if size < int64(bspHeaderSize) {
		return nil, fmt.Errorf("BSP too small: %d bytes", size)
	}

	// Read header
	header := make([]byte, bspHeaderSize)
	if _, err := r.ReadAt(header, 0); err != nil {
		return nil, fmt.Errorf("read BSP header: %w", err)
	}

	// Verify magic and version
	if string(header[0:4]) != bspMagic {
		return nil, fmt.Errorf("invalid BSP magic: %q", header[0:4])
	}
	version := binary.LittleEndian.Uint32(header[4:8])
	if version != bspVersion {
		return nil, fmt.Errorf("unsupported BSP version: %d", version)
	}

	assets := &BSPAssets{}

	// Parse entities lump
	entOffset := int64(binary.LittleEndian.Uint32(header[8+bspLumpEntities*8:]))
	entLength := int64(binary.LittleEndian.Uint32(header[8+bspLumpEntities*8+4:]))
	if entLength > 0 {
		entData := make([]byte, entLength)
		if _, err := r.ReadAt(entData, entOffset); err != nil {
			return nil, fmt.Errorf("read entities lump: %w", err)
		}
		parseEntities(string(entData), assets)
	}

	// Parse shaders lump
	shaderOffset := int64(binary.LittleEndian.Uint32(header[8+bspLumpShaders*8:]))
	shaderLength := int64(binary.LittleEndian.Uint32(header[8+bspLumpShaders*8+4:]))
	numShaders := shaderLength / bspShaderSize
	if numShaders > 0 {
		shaderData := make([]byte, shaderLength)
		if _, err := r.ReadAt(shaderData, shaderOffset); err != nil {
			return nil, fmt.Errorf("read shaders lump: %w", err)
		}
		for i := int64(0); i < numShaders; i++ {
			nameBytes := shaderData[i*bspShaderSize : i*bspShaderSize+64]
			name := strings.ReplaceAll(readNullTerminated(nameBytes), "\\", "/")
			if name != "" && !strings.HasPrefix(name, "*") {
				assets.Shaders = append(assets.Shaders, name)
			}
		}
	}

	return assets, nil
}

// parseEntities extracts asset refs from BSP entity text.
func parseEntities(text string, assets *BSPAssets) {
	scanner := strings.NewReader(text)
	buf := make([]byte, len(text))
	n, _ := scanner.Read(buf)
	lines := strings.Split(string(buf[:n]), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "{" || line == "}" {
			continue
		}

		// Parse key-value pair: "key" "value"
		key, value := parseEntityKV(line)
		if key == "" {
			continue
		}

		// Normalize Windows backslashes to forward slashes
		value = strings.ReplaceAll(value, "\\", "/")

		switch strings.ToLower(key) {
		case "music":
			// Music value can contain a space-separated looping flag
			parts := strings.Fields(value)
			if len(parts) > 0 && parts[0] != "" {
				assets.Music = append(assets.Music, parts[0])
			}
		case "noise":
			if value != "" && !strings.HasPrefix(value, "*") {
				assets.Sounds = append(assets.Sounds, value)
			}
		case "model2":
			if value != "" && !strings.HasPrefix(value, "*") {
				assets.Models = append(assets.Models, value)
			}
		}
	}
}

// parseEntityKV parses a "key" "value" line from entity data.
func parseEntityKV(line string) (string, string) {
	// Find first quoted string
	i := strings.IndexByte(line, '"')
	if i < 0 {
		return "", ""
	}
	j := strings.IndexByte(line[i+1:], '"')
	if j < 0 {
		return "", ""
	}
	key := line[i+1 : i+1+j]

	// Find second quoted string
	rest := line[i+1+j+1:]
	i = strings.IndexByte(rest, '"')
	if i < 0 {
		return key, ""
	}
	j = strings.IndexByte(rest[i+1:], '"')
	if j < 0 {
		return key, ""
	}
	value := rest[i+1 : i+1+j]

	return key, value
}

// readNullTerminated reads a null-terminated string from a byte slice.
func readNullTerminated(b []byte) string {
	idx := bytes.IndexByte(b, 0)
	if idx < 0 {
		return string(b)
	}
	return string(b[:idx])
}
