package assets

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteMobilePak writes a reduced copy of a Trinity pak for memory-constrained
// (mobile) web clients: image entries whose path is already present in the
// game's baseline pk3 are omitted, so the engine's VFS falls back to the
// smaller stock copies. Server-only bot nav data (.aat) is dropped too.
func WriteMobilePak(srcPk3, dstPath string, baseline map[string]bool) error {
	r, err := zip.OpenReader(srcPk3)
	if err != nil {
		return fmt.Errorf("open %s: %w", srcPk3, err)
	}
	defer r.Close()

	out, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", dstPath, err)
	}
	w := zip.NewWriter(out)

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		lower := strings.ToLower(f.Name)
		if isImagePath(lower) && baseline[lower] {
			continue
		}
		if strings.HasSuffix(lower, ".aat") {
			continue
		}
		if err := w.Copy(f); err != nil {
			w.Close()
			out.Close()
			return fmt.Errorf("copy %s: %w", f.Name, err)
		}
	}

	if err := w.Close(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func isImagePath(lowerPath string) bool {
	switch filepath.Ext(lowerPath) {
	case ".tga", ".jpg", ".jpeg", ".png":
		return true
	}
	return false
}

// RefreshMobilePak rewrites trinityPak's mobile copy in demoDir against the
// baseline the last demo bake wrote there, so a mod update needs no new bake.
// The copy is renamed into place, so a client never downloads a partial one.
func RefreshMobilePak(demoDir, game, trinityPak string) (string, error) {
	r, err := zip.OpenReader(filepath.Join(demoDir, game+".pk3"))
	if err != nil {
		return "", fmt.Errorf("read the %s baseline (run trinity demobake first): %w", game, err)
	}
	baseline := make(map[string]bool, len(r.File))
	for _, f := range r.File {
		if !f.FileInfo().IsDir() {
			baseline[strings.ToLower(f.Name)] = true
		}
	}
	r.Close()

	base := filepath.Base(trinityPak)
	out := filepath.Join(demoDir, strings.TrimSuffix(base, filepath.Ext(base))+"-mobile.pk3")
	tmp := out + ".tmp"
	if err := WriteMobilePak(trinityPak, tmp, baseline); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, out); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return out, nil
}

// RefreshMobilePaks rewrites the mobile copy of each game's installed Trinity
// pak, picking the pak the demo bake would.
func RefreshMobilePaks(quake3Dir, demoDir string) ([]string, error) {
	gamePk3s := CollectGamePk3s(quake3Dir)
	var written []string
	for _, game := range []string{"baseq3", "missionpack"} {
		var trinityPak string
		for _, pk3 := range gamePk3s[game] {
			if IsTrinityPak(pk3) {
				trinityPak = pk3
			}
		}
		if trinityPak == "" {
			continue
		}
		out, err := RefreshMobilePak(demoDir, game, trinityPak)
		if err != nil {
			return written, err
		}
		written = append(written, out)
	}
	return written, nil
}
