package assets

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CollectGamePk3s returns game dir name → ordered pk3 paths for each game directory
// found under quake3Dir (e.g. "baseq3", "missionpack").
func CollectGamePk3s(quake3Dir string) map[string][]string {
	result := make(map[string][]string)
	for _, subdir := range []string{"baseq3", "missionpack"} {
		dir := filepath.Join(quake3Dir, subdir)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		files := CollectPk3sInDir(dir)
		if len(files) > 0 {
			result[subdir] = files
		}
	}
	return result
}

// CollectPk3sInDir returns the pk3s directly inside dir in the engine's load
// order (FS_AddGameDirectory), so a later pk3 takes priority over an earlier one.
func CollectPk3sInDir(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".pk3") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.SliceStable(files, func(i, j int) bool {
		return fsPathCmp(filepath.Base(files[i]), filepath.Base(files[j])) < 0
	})
	return files
}

// fsPathCmp mirrors the engine's FS_PathCmp, which folds letters to upper
// case, so '_' sorts after every letter.
func fsPathCmp(a, b string) int {
	for i := 0; ; i++ {
		var ca, cb byte
		if i < len(a) {
			ca = fsFold(a[i])
		}
		if i < len(b) {
			cb = fsFold(b[i])
		}
		if ca != cb {
			if ca < cb {
				return -1
			}
			return 1
		}
		if ca == 0 {
			return 0
		}
	}
}

func fsFold(c byte) byte {
	switch {
	case c >= 'a' && c <= 'z':
		return c - ('a' - 'A')
	case c == '\\' || c == ':':
		return '/'
	}
	return c
}

// ReadFileFromPk3 reads a single file from a pk3 archive.
func ReadFileFromPk3(pk3Path, virtualPath string) ([]byte, error) {
	r, err := zip.OpenReader(pk3Path)
	if err != nil {
		return nil, fmt.Errorf("open pk3 %s: %w", pk3Path, err)
	}
	defer r.Close()

	lowerTarget := strings.ToLower(virtualPath)
	for _, f := range r.File {
		if strings.ToLower(f.Name) == lowerTarget {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open %s in %s: %w", virtualPath, pk3Path, err)
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("%s not found in %s", virtualPath, pk3Path)
}

// WritePk3 creates a pk3 (zip) file with the given files using Deflate compression.
func WritePk3(outputPath string, files map[string][]byte) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outputPath, err)
	}
	defer f.Close()

	return WritePk3ToWriter(f, files)
}

// WritePk3ToWriter writes a pk3 (zip) to the given writer using Deflate compression.
func WritePk3ToWriter(w io.Writer, files map[string][]byte) error {
	zw := zip.NewWriter(w)

	// Sort keys for deterministic output
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		header := &zip.FileHeader{
			Name:   name,
			Method: zip.Deflate,
		}
		fw, err := zw.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("create entry %s: %w", name, err)
		}
		if _, err := fw.Write(files[name]); err != nil {
			return fmt.Errorf("write entry %s: %w", name, err)
		}
	}

	return zw.Close()
}

// IteratePk3 iterates over entries in a pk3 file, calling fn for each entry.
func IteratePk3(pk3Path string, fn func(name string, open func() (io.ReadCloser, error)) error) error {
	r, err := zip.OpenReader(pk3Path)
	if err != nil {
		return fmt.Errorf("open pk3 %s: %w", pk3Path, err)
	}
	defer r.Close()

	for _, f := range r.File {
		if err := fn(f.Name, f.Open); err != nil {
			return err
		}
	}
	return nil
}

// BuildFileIndex builds a case-insensitive file index across all pk3s for a game.
// Later pk3s override earlier ones. Returns lowered path → source pk3 path.
func BuildFileIndex(pk3Paths []string) (map[string]string, error) {
	index := make(map[string]string)
	for _, pk3Path := range pk3Paths {
		r, err := zip.OpenReader(pk3Path)
		if err != nil {
			return nil, fmt.Errorf("open pk3 %s: %w", pk3Path, err)
		}
		for _, f := range r.File {
			if f.FileInfo().IsDir() {
				continue
			}
			index[strings.ToLower(f.Name)] = pk3Path
		}
		r.Close()
	}
	return index, nil
}

// IsOfficialPak returns true if the filename matches pak[0-9].pk3 (official id Software paks).
// Excludes pak[0-9]t.pk3 (Trinity override paks).
func IsOfficialPak(filename string) bool {
	lower := strings.ToLower(filepath.Base(filename))
	if len(lower) != 8 {
		return false
	}
	return strings.HasPrefix(lower, "pak") && lower[3] >= '0' && lower[3] <= '9' && lower[4:] == ".pk3"
}

// IsTrinityPak returns true if the filename matches pak[0-9]t.pk3 (Trinity override paks).
func IsTrinityPak(filename string) bool {
	lower := strings.ToLower(filepath.Base(filename))
	if len(lower) != 9 {
		return false
	}
	return strings.HasPrefix(lower, "pak") && lower[3] >= '0' && lower[3] <= '9' && lower[4:] == "t.pk3"
}

// ExtractFilesFromPk3s extracts specified files from pk3s using the file index.
// Returns path → file data for all files found.
func ExtractFilesFromPk3s(paths []string, fileIndex map[string]string) (map[string][]byte, error) {
	// Group by source pk3
	byPk3 := make(map[string][]string)
	for _, path := range paths {
		lower := strings.ToLower(path)
		pk3, ok := fileIndex[lower]
		if !ok {
			continue
		}
		byPk3[pk3] = append(byPk3[pk3], lower)
	}

	result := make(map[string][]byte)

	for pk3Path, wantedPaths := range byPk3 {
		wanted := make(map[string]bool, len(wantedPaths))
		for _, p := range wantedPaths {
			wanted[p] = true
		}

		r, err := zip.OpenReader(pk3Path)
		if err != nil {
			return nil, fmt.Errorf("open pk3 %s: %w", pk3Path, err)
		}

		for _, f := range r.File {
			lower := strings.ToLower(f.Name)
			if !wanted[lower] {
				continue
			}
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
			result[lower] = data
			delete(wanted, lower)
		}
		r.Close()
	}

	return result, nil
}
