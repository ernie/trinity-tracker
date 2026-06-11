package assets

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"strings"
)

// BuildMapPak builds a per-map pk3 containing all map-specific assets not in
// the baseline. One artifact serves every game mount, and maps installed in
// baseq3 may depend on missionpack assets (and vice versa via the merged
// index), so dependencies are resolved under each game's manifest and the
// union is bundled, minus each game's own baseline.
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

		// The BSP bytes are identical under every game (the merged
		// missionpack index points at the same source pk3) — parse once.
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

	// Resolve entity sounds. BSP "noise" values often omit the
	// extension (e.g. "sound/ct3tourney2/thunder"); the engine tries
	// .wav then .ogg at load time (snd_codec.c:S_CodecGetSound). Mirror
	// that so the file actually gets bundled — otherwise the engine
	// falls back to default_sfx, which is sound/feedback/hit.wav, and
	// looped speakers loop the hit sound forever.
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

// resolveShaderTextures resolves a shader name to its texture dependencies and adds them to needed.
// The engine strips any extension before shader lookup (R_FindShader →
// COM_StripExtension), so an MD3 ref like "textures/x/flag_0.tga" matches
// a shader script's "textures/x/flag_0" — mirror that here.
func resolveShaderTextures(shaderName string, gm *GameManifest, needed map[string]bool) {
	lower := stripExtension(strings.ToLower(shaderName))

	// Look up shader definition
	if textures, ok := gm.Shaders[lower]; ok {
		for _, tex := range textures {
			if resolved, ok := ResolveTexture(tex, gm.FileIndex); ok {
				needed[resolved] = true
			}
		}
		// If shader def has no texture refs (e.g. only surfaceparms),
		// the engine uses the shader name as an implicit texture
		if len(textures) == 0 {
			if resolved, ok := ResolveTexture(lower, gm.FileIndex); ok {
				needed[resolved] = true
			}
		}
		// Include the .shader script file so the engine can find the definition
		if scriptPath, ok := gm.ShaderFiles[lower]; ok {
			needed[scriptPath] = true
		}
	} else {
		// No shader def — treat as direct texture path
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

	// Only .md3 has a shader-table format we can parse; iqm/mdr embed
	// materials differently and aren't worth a parser here — bundling
	// the file itself is the important part.
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

// resolveAudioPath finds an audio asset in the file index, trying the
// engine's extension-fallback order if the literal path isn't present.
// Q3's snd_codec.c tries the given filename first, then strips the
// extension and tries each codec — wav before ogg, since wav is
// registered last and the codec list is a LIFO stack.
func resolveAudioPath(soundPath string, fileIndex map[string]string) (string, bool) {
	lower := strings.ToLower(soundPath)
	if _, ok := fileIndex[lower]; ok {
		return lower, true
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
