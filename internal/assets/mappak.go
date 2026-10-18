package assets

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"strings"
)

// BuildMapPak builds a per-map pk3 of the map's assets that aren't in the
// baseline. One pk3 serves every game mount, and a map may use assets from
// a game it isn't installed in, so it bundles the union of dependencies
// across game manifests, minus each game's baseline.
func BuildMapPak(mapName string, games []string, manifest *Manifest, outputPath string) error {
	lowerBSP := strings.ToLower("maps/" + mapName + ".bsp")

	var bspAssets *BSPAssets
	union := make(map[string]string) // lowered path → source pk3

	for _, game := range games {
		gm, ok := manifest.Games[game]
		if !ok {
			continue
		}
		if _, ok := gm.FileIndex[lowerBSP]; !ok {
			continue
		}

		// The BSP is the same under every game, so it's parsed once.
		if bspAssets == nil {
			bspData, err := readFileFromIndex(lowerBSP, gm.FileIndex)
			if err != nil {
				return fmt.Errorf("read BSP: %w", err)
			}
			bspAssets, err = ParseBSP(bytes.NewReader(bspData), int64(len(bspData)))
			if err != nil {
				return fmt.Errorf("parse BSP: %w", err)
			}
			log.Printf("  %s: BSP has %d shaders, %d models, %d sounds, %d music",
				mapName, len(bspAssets.Shaders), len(bspAssets.Models), len(bspAssets.Sounds), len(bspAssets.Music))
		}

		for path := range resolveMapNeeds(mapName, lowerBSP, bspAssets, gm) {
			if gm.BaselineFiles[path] {
				continue
			}
			union[path] = gm.FileIndex[path]
		}
	}

	if bspAssets == nil {
		return fmt.Errorf("BSP not found: maps/%s.bsp", mapName)
	}

	if len(union) == 0 {
		log.Printf("  %s: no non-baseline files needed", mapName)
		return nil
	}

	// Extract and write
	paths := make([]string, 0, len(union))
	for p := range union {
		paths = append(paths, p)
	}

	files, err := ExtractFilesFromPk3s(paths, union)
	if err != nil {
		return fmt.Errorf("extract files: %w", err)
	}

	if err := WritePk3(outputPath, files); err != nil {
		return fmt.Errorf("write map pk3: %w", err)
	}

	log.Printf("  %s: %d files", mapName, len(files))
	return nil
}

// resolveMapNeeds collects every file the map needs under one game's manifest.
func resolveMapNeeds(mapName, lowerBSP string, bspAssets *BSPAssets, gm *GameManifest) map[string]bool {
	needed := map[string]bool{lowerBSP: true}

	// Resolve BSP surface shaders
	for _, shaderName := range bspAssets.Shaders {
		resolveShaderTextures(shaderName, gm, needed)
	}

	// Resolve entity models (model2)
	for _, modelPath := range bspAssets.Models {
		resolveModel(modelPath, gm, needed)
	}

	// BSP "noise" values often omit the extension (such as
	// "sound/ct3tourney2/thunder"), and the engine tries .wav, then .ogg
	// (S_CodecGetSound in snd_codec.c). Resolving the same way bundles
	// the real file; a missing one plays default_sfx,
	// sound/feedback/hit.wav, which looped speakers repeat forever.
	for _, soundPath := range bspAssets.Sounds {
		if resolved, ok := resolveAudioPath(soundPath, gm.FileIndex); ok {
			needed[resolved] = true
		}
	}

	// Resolve music (same extension-fallback rules as entity sounds)
	for _, musicPath := range bspAssets.Music {
		if resolved, ok := resolveAudioPath(musicPath, gm.FileIndex); ok {
			needed[resolved] = true
		}
	}

	// Include levelshot
	for _, ext := range []string{".jpg", ".tga"} {
		ls := "levelshots/" + mapName + ext
		if _, ok := gm.FileIndex[ls]; ok {
			needed[ls] = true
			break
		}
	}

	// Include arena file
	arenaPath := "scripts/" + mapName + ".arena"
	if _, ok := gm.FileIndex[arenaPath]; ok {
		needed[arenaPath] = true
	}

	return needed
}

// resolveShaderTextures adds a shader's texture dependencies to needed.
// The engine strips any extension before the shader lookup (R_FindShader
// calls COM_StripExtension), so an MD3 ref like "textures/x/flag_0.tga"
// matches a shader script's "textures/x/flag_0"; this does the same.
func resolveShaderTextures(shaderName string, gm *GameManifest, needed map[string]bool) {
	lower := stripExtension(strings.ToLower(shaderName))

	if textures, ok := gm.Shaders[lower]; ok {
		for _, tex := range textures {
			if resolved, ok := ResolveTexture(tex, gm.FileIndex); ok {
				needed[resolved] = true
			}
		}
		// A shader with no texture refs (only surfaceparms, say) uses
		// its name as the texture.
		if len(textures) == 0 {
			if resolved, ok := ResolveTexture(lower, gm.FileIndex); ok {
				needed[resolved] = true
			}
		}
		if scriptPath, ok := gm.ShaderFiles[lower]; ok {
			needed[scriptPath] = true
		}
	} else {
		// With no shader, the name is a texture path.
		if resolved, ok := ResolveTexture(lower, gm.FileIndex); ok {
			needed[resolved] = true
		}
	}
}

// stripExtension removes a trailing extension, mirroring the engine's
// COM_StripExtension (only a dot after the last slash counts).
func stripExtension(p string) string {
	if i := strings.LastIndexByte(p, '.'); i > strings.LastIndexByte(p, '/') {
		return p[:i]
	}
	return p
}

// resolveModel resolves a model and all its shader/texture dependencies.
// BSP "model2" values usually include the extension, but mirror the
// engine's fallback order (renderer/tr_model.c modelLoaders: iqm, mdr,
// md3) for completeness so a custom map referencing a bare path doesn't
// silently drop the model from the bundle.
func resolveModel(modelPath string, gm *GameManifest, needed map[string]bool) {
	resolved, ok := resolveModelPath(modelPath, gm.FileIndex)
	if !ok {
		return
	}
	needed[resolved] = true

	// Only .md3 shader tables are parsed; for iqm and mdr, bundling the
	// model file is enough.
	if !strings.HasSuffix(resolved, ".md3") {
		return
	}

	data, err := readFileFromIndex(resolved, gm.FileIndex)
	if err != nil {
		return
	}
	shaderRefs, err := ParseMD3Shaders(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return
	}

	for _, ref := range shaderRefs {
		resolveShaderTextures(ref, gm, needed)
	}
}

// resolveModelPath finds a model in the file index, trying the engine's
// extension-fallback order (iqm, mdr, md3) when the literal path isn't
// present. Matches modelLoaders[] in renderer/tr_model.c.
func resolveModelPath(modelPath string, fileIndex map[string]string) (string, bool) {
	lower := strings.ToLower(modelPath)
	if _, ok := fileIndex[lower]; ok {
		return lower, true
	}
	base := lower
	for _, ext := range []string{".md3", ".mdr", ".iqm"} {
		base = strings.TrimSuffix(base, ext)
	}
	for _, ext := range []string{".iqm", ".mdr", ".md3"} {
		candidate := base + ext
		if candidate == lower {
			continue
		}
		if _, ok := fileIndex[candidate]; ok {
			return candidate, true
		}
	}
	return "", false
}

// The engine appends each codec extension to the full reference name
// (x.ogg -> x.ogg.wav), so resolve that before the stripped-base form.
func resolveAudioPath(soundPath string, fileIndex map[string]string) (string, bool) {
	lower := strings.ToLower(soundPath)
	if _, ok := fileIndex[lower]; ok {
		return lower, true
	}
	for _, ext := range []string{".wav", ".ogg"} {
		if _, ok := fileIndex[lower+ext]; ok {
			return lower + ext, true
		}
	}
	base := strings.TrimSuffix(lower, ".wav")
	base = strings.TrimSuffix(base, ".ogg")
	for _, ext := range []string{".wav", ".ogg"} {
		candidate := base + ext
		if candidate == lower {
			continue
		}
		if _, ok := fileIndex[candidate]; ok {
			return candidate, true
		}
	}
	return "", false
}

// MapPakFileSet returns the set of files in a map pk3 by reading it.
func MapPakFileSet(mapPk3Path string) (map[string]bool, error) {
	fileSet := make(map[string]bool)
	err := IteratePk3(mapPk3Path, func(name string, open func() (io.ReadCloser, error)) error {
		fileSet[strings.ToLower(name)] = true
		return nil
	})
	return fileSet, err
}
