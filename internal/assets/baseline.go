package assets

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Prefix whitelist for baseline pk3 files
var baselinePrefixes = []string{
	"gfx/",
	"sprites/",
	"icons/",
	"fonts/",
	"menu/",
	"ui/",
	"botfiles/",
	"models/weapons/",
	"models/weapons2/",
	"models/weaphits/",
	"models/powerups/",
	"models/mapobjects/",
	"models/flags/",
	"models/ammo/",
	"models/gibs/",
	"models/misc/",
	"sound/",
	"scripts/",
	"vm/",
	"textures/sfx/",
	"textures/effects/",
	"textures/sfx2/",
	"textures/effects2/",
	"textures/ctf2/",
	"team_icon/",
	"models/players/",
}

// baselineExcludePrefixes are prefixes explicitly excluded from baseline
var baselineExcludePrefixes = []string{
	"textures/",
	"maps/",
	"env/",
	"levelshots/",
	"demos/",
	"video/",
	"music/",
	"models/players/",
}

// BuildBaseline builds baseline pk3s, Trinity pk3 copies, manifest, and all map pk3s.
func BuildBaseline(quake3Dir, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(outputDir, "maps"), 0755); err != nil {
		return fmt.Errorf("create maps dir: %w", err)
	}

	gamePk3s := CollectGamePk3s(quake3Dir)
	if len(gamePk3s) == 0 {
		return fmt.Errorf("no game directories found in %s", quake3Dir)
	}

	manifest := &Manifest{
		Games: make(map[string]*GameManifest),
	}

	// Process each game directory
	for _, game := range []string{"baseq3", "missionpack"} {
		pk3s, ok := gamePk3s[game]
		if !ok {
			continue
		}

		log.Printf("Processing %s (%d pk3s)...", game, len(pk3s))

		gm, err := buildGameBaseline(game, pk3s, outputDir)
		if err != nil {
			return fmt.Errorf("build %s baseline: %w", game, err)
		}
		manifest.Games[game] = gm
	}

	// For missionpack, merge baseq3 file index underneath (baseq3 as base, missionpack overrides)
	if mp, ok := manifest.Games["missionpack"]; ok {
		if bq3, ok := manifest.Games["baseq3"]; ok {
			merged := make(map[string]string, len(bq3.FileIndex)+len(mp.FileIndex))
			for k, v := range bq3.FileIndex {
				merged[k] = v
			}
			for k, v := range mp.FileIndex {
				merged[k] = v
			}
			mp.FileIndex = merged

			// Merge shaders too
			mergedShaders := make(map[string][]string, len(bq3.Shaders)+len(mp.Shaders))
			for k, v := range bq3.Shaders {
				mergedShaders[k] = v
			}
			for k, v := range mp.Shaders {
				mergedShaders[k] = v
			}
			mp.Shaders = mergedShaders

			// Merge shader files
			mergedShaderFiles := make(map[string]string, len(bq3.ShaderFiles)+len(mp.ShaderFiles))
			for k, v := range bq3.ShaderFiles {
				mergedShaderFiles[k] = v
			}
			for k, v := range mp.ShaderFiles {
				mergedShaderFiles[k] = v
			}
			mp.ShaderFiles = mergedShaderFiles

			// Merge baseline files
			mergedBaseline := make(map[string]bool, len(bq3.BaselineFiles)+len(mp.BaselineFiles))
			for k := range bq3.BaselineFiles {
				mergedBaseline[k] = true
			}
			for k := range mp.BaselineFiles {
				mergedBaseline[k] = true
			}
			mp.BaselineFiles = mergedBaseline
		}
	}

	// Save manifest
	manifestPath := filepath.Join(outputDir, "manifest.json")
	if err := manifest.Save(manifestPath); err != nil {
		return fmt.Errorf("save manifest: %w", err)
	}
	log.Printf("Manifest saved to %s", manifestPath)

	// Pre-build all map pk3s.
	var games []string
	for _, game := range []string{"baseq3", "missionpack"} {
		if _, ok := manifest.Games[game]; ok {
			games = append(games, game)
		}
	}

	builtMaps := make(map[string]bool)
	for _, game := range games {
		for path := range manifest.Games[game].FileIndex {
			if !strings.HasPrefix(path, "maps/") || !strings.HasSuffix(path, ".bsp") {
				continue
			}
			mapName := strings.TrimSuffix(strings.TrimPrefix(path, "maps/"), ".bsp")
			if builtMaps[mapName] {
				continue
			}
			builtMaps[mapName] = true
			mapPk3Path := filepath.Join(outputDir, "maps", mapName+".pk3")
			log.Printf("Building map pk3: %s", mapName)
			if err := BuildMapPak(mapName, games, manifest, mapPk3Path); err != nil {
				log.Printf("Warning: failed to build map pk3 for %s: %v", mapName, err)
			}
		}
	}

	return nil
}

func buildGameBaseline(game string, pk3s []string, outputDir string) (*GameManifest, error) {
	// Build file index across ALL pk3s
	fileIndex, err := BuildFileIndex(pk3s)
	if err != nil {
		return nil, fmt.Errorf("build file index: %w", err)
	}

	// Identify official pak files and Trinity pak files
	var officialPaks []string
	var trinityPak string
	for _, pk3Path := range pk3s {
		base := filepath.Base(pk3Path)
		if IsOfficialPak(base) {
			officialPaks = append(officialPaks, pk3Path)
		}
		if IsTrinityPak(base) {
			trinityPak = pk3Path
		}
	}

	// Build baseline from official paks only
	baselineFiles := make(map[string][]byte)
	for _, pk3Path := range officialPaks {
		r, err := zip.OpenReader(pk3Path)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", pk3Path, err)
		}

		for _, f := range r.File {
			if f.FileInfo().IsDir() {
				continue
			}
			lower := strings.ToLower(f.Name)
			if isBaselineFile(lower) {
				rc, err := f.Open()
				if err != nil {
					r.Close()
					return nil, fmt.Errorf("open %s in %s: %w", f.Name, pk3Path, err)
				}
				data, err := io.ReadAll(rc)
				rc.Close()
				if err != nil {
					r.Close()
					return nil, fmt.Errorf("read %s in %s: %w", f.Name, pk3Path, err)
				}
				baselineFiles[lower] = data
			}
		}
		r.Close()
	}

	// Write baseline pk3
	outputName := game + ".pk3"
	outputPath := filepath.Join(outputDir, outputName)
	if err := WritePk3(outputPath, baselineFiles); err != nil {
		return nil, fmt.Errorf("write baseline pk3: %w", err)
	}

	info, _ := os.Stat(outputPath)
	log.Printf("  %s: %d files, %.1f MB", outputName, len(baselineFiles), float64(info.Size())/(1024*1024))

	// Track baseline file set
	baselineSet := make(map[string]bool, len(baselineFiles))
	for path := range baselineFiles {
		baselineSet[path] = true
	}

	// Add Trinity pk3 contents to baseline set (loaded separately by demo player)
	if trinityPak != "" {
		r, err := zip.OpenReader(trinityPak)
		if err == nil {
			for _, f := range r.File {
				if !f.FileInfo().IsDir() {
					baselineSet[strings.ToLower(f.Name)] = true
				}
			}
			r.Close()
		}
		log.Printf("  %s: %d files added to baseline set", filepath.Base(trinityPak), len(baselineSet)-len(baselineFiles))
	}

	// Parse all shaders from all pk3s (in load order)
	shaders := make(map[string][]string)
	shaderFiles := make(map[string]string)
	for _, pk3Path := range pk3s {
		if err := parseShadersPk3(pk3Path, shaders, shaderFiles); err != nil {
			log.Printf("Warning: failed to parse shaders from %s: %v", filepath.Base(pk3Path), err)
		}
	}
	log.Printf("  %d shader definitions parsed", len(shaders))

	return &GameManifest{
		FileIndex:     fileIndex,
		BaselineFiles: baselineSet,
		Shaders:       shaders,
		ShaderFiles:   shaderFiles,
	}, nil
}

func isBaselineFile(lowerPath string) bool {
	// Check specific includes first (these override broad excludes)
	for _, prefix := range baselinePrefixes {
		if strings.HasPrefix(lowerPath, prefix) {
			return true
		}
	}

	// Check excludes
	for _, prefix := range baselineExcludePrefixes {
		if strings.HasPrefix(lowerPath, prefix) {
			return false
		}
	}

	// Root-level .cfg files
	if !strings.Contains(lowerPath, "/") && strings.HasSuffix(lowerPath, ".cfg") {
		return true
	}

	return false
}

func parseShadersPk3(pk3Path string, shaders map[string][]string, shaderFiles map[string]string) error {
	return IteratePk3(pk3Path, func(name string, open func() (io.ReadCloser, error)) error {
		lower := strings.ToLower(name)
		if !strings.HasPrefix(lower, "scripts/") || !strings.HasSuffix(lower, ".shader") {
			return nil
		}

		rc, err := open()
		if err != nil {
			return nil
		}
		defer rc.Close()

		defs, err := ParseShaderScript(rc)
		if err != nil {
			return nil
		}

		for _, def := range defs {
			key := strings.ToLower(def.Name)
			shaders[key] = def.Textures
			shaderFiles[key] = lower
		}
		return nil
	})
}

// readFileFromIndex reads a file using the file index to locate its source pk3.
func readFileFromIndex(path string, fileIndex map[string]string) ([]byte, error) {
	lower := strings.ToLower(path)
	pk3Path, ok := fileIndex[lower]
	if !ok {
		return nil, fmt.Errorf("file not in index: %s", path)
	}
	return ReadFileFromPk3(pk3Path, lower)
}

// readFileAsReaderAt reads a file from index and returns a bytes.Reader for ReaderAt support.
func readFileAsReaderAt(path string, fileIndex map[string]string) (*bytes.Reader, error) {
	data, err := readFileFromIndex(path, fileIndex)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}
