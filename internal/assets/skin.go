package assets

import (
	"bufio"
	"io"
	"strings"
)

// ParseSkin parses a Q3 .skin file (comma-separated surface,texture_path lines).
// Returns the list of non-empty texture paths.
func ParseSkin(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	var textures []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		// Format: surface,texture_path
		parts := strings.SplitN(line, ",", 2)
		if len(parts) < 2 {
			continue
		}
		path := strings.TrimSpace(parts[1])
		if path != "" {
			textures = append(textures, path)
		}
	}

	return textures, scanner.Err()
}

// SkinEntry maps one model surface to its texture path.
type SkinEntry struct {
	Surface string
	Texture string
}

// ParseSkinMap parses a Q3 .skin file into ordered surface→texture entries,
// keeping the surface name each maps to.
func ParseSkinMap(r io.Reader) ([]SkinEntry, error) {
	scanner := bufio.NewScanner(r)
	var entries []SkinEntry

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		parts := strings.SplitN(line, ",", 2)
		if len(parts) < 2 {
			continue
		}
		surface := strings.TrimSpace(parts[0])
		texture := strings.TrimSpace(parts[1])
		if surface == "" || texture == "" {
			continue
		}
		entries = append(entries, SkinEntry{Surface: surface, Texture: texture})
	}

	return entries, scanner.Err()
}
