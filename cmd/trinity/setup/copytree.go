package setup

import (
	"io"
	"os"
	"path/filepath"
)

// CopyTree mirrors src into dst, preserving file modes and symlinks.
// Existing files in dst are overwritten; nothing in dst is removed —
// the destination may already hold runtime-extracted assets
// (levelshots, demopk3s) that we must not blow away.
//
// Ownership of created entries follows the calling process — used as
// the in-Go body of `_helper copy`, which re-execs trinity with the
// service user's credentials so every file lands service-owned at
// creation rather than via a follow-up chown.
func CopyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_ = os.Remove(target)
			return os.Symlink(link, target)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}
