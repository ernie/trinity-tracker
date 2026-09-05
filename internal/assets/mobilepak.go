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
