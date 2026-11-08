package assets

import (
	"encoding/json"
	"fmt"
	"os"
)

// Manifest caches file index, baseline file set, and shader definitions
// to avoid re-scanning pk3s for map and demo pk3 builders.
type Manifest struct {
	Games map[string]*GameManifest `json:"games"`
}

// GameManifest holds per-game manifest data.
type GameManifest struct {
	FileIndex     map[string]string      `json:"fileIndex"`     // lowered path → source pk3
	BaselineFiles map[string]bool        `json:"baselineFiles"` // paths in baseline + trinity pk3s
	Shaders       map[string]ShaderEntry `json:"shaders"`       // shader name → definition the engine uses
}

// LoadManifest loads a manifest from a JSON file.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}

// Save writes the manifest to a JSON file.
func (m *Manifest) Save(path string) error {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}
