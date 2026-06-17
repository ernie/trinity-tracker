package assets

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SplitOptions struct {
	Sources       []string
	Quake3Dir     string
	BaselineGames []string // empty defaults to baseq3 only
	OutputDir     string
	Prefix        string
	DryRun        bool
}

type MapReport struct {
	Name         string
	FileCount    int
	Bytes        int64
	MissingAudio []string
}

type SplitReport struct {
	Maps       []MapReport
	Failed     []string
	DeadFiles  int
	DeadBytes  int64
	TotalBytes int64
}

type srcEntry struct {
	pk3   string
	crc   uint32
	bytes int64
}

// Guard against bundling id Software's copyrighted assets.
func stockBaseline(quake3Dir string, games []string) (map[string]uint32, error) {
	allPaks := CollectGamePk3s(quake3Dir)
	var stockPaks []string
	for _, game := range games {
		for _, p := range allPaks[game] {
			if IsOfficialPak(p) {
				stockPaks = append(stockPaks, p)
			}
		}
	}
	if len(stockPaks) == 0 {
		return nil, fmt.Errorf("no official pak[0-9].pk3 found under %s for games %v — refusing to run", quake3Dir, games)
	}

	crc := make(map[string]uint32)
	for _, p := range stockPaks {
		r, err := zip.OpenReader(p)
		if err != nil {
			return nil, fmt.Errorf("open stock pak %s: %w", p, err)
		}
		for _, f := range r.File {
			if !f.FileInfo().IsDir() {
				crc[strings.ToLower(f.Name)] = f.CRC32
			}
		}
		r.Close()
	}
	return crc, nil
}

func scanSources(sources []string) (map[string]srcEntry, error) {
	index := make(map[string]srcEntry)
	for _, p := range sources {
		r, err := zip.OpenReader(p)
		if err != nil {
			return nil, fmt.Errorf("open source %s: %w", p, err)
		}
		for _, f := range r.File {
			if f.FileInfo().IsDir() {
				continue
			}
			index[strings.ToLower(f.Name)] = srcEntry{p, f.CRC32, int64(f.CompressedSize64)}
		}
		r.Close()
	}
	return index, nil
}

// SplitMapPack writes one standalone pk3 per map, carrying every non-stock
// asset the map references plus its .aas botfile.
func SplitMapPack(opts SplitOptions) (*SplitReport, error) {
	games := opts.BaselineGames
	if len(games) == 0 {
		games = []string{"baseq3"}
	}
	stock, err := stockBaseline(opts.Quake3Dir, games)
	if err != nil {
		return nil, err
	}
	src, err := scanSources(opts.Sources)
	if err != nil {
		return nil, err
	}

	fileIndex := make(map[string]string, len(src))
	for path, e := range src {
		fileIndex[path] = e.pk3
	}

	stockIndex := make(map[string]string, len(stock))
	for path := range stock {
		stockIndex[path] = ""
	}

	shaders := make(map[string][]string)
	shaderFiles := make(map[string]string)
	for _, p := range opts.Sources {
		if err := parseShadersPk3(p, shaders, shaderFiles); err != nil {
			return nil, fmt.Errorf("parse shaders %s: %w", p, err)
		}
	}
	gm := &GameManifest{FileIndex: fileIndex, Shaders: shaders, ShaderFiles: shaderFiles}

	var names []string
	for path := range src {
		if strings.HasPrefix(path, "maps/") && strings.HasSuffix(path, ".bsp") {
			names = append(names, strings.TrimSuffix(strings.TrimPrefix(path, "maps/"), ".bsp"))
		}
	}
	sort.Strings(names)

	if !opts.DryRun {
		if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
			return nil, fmt.Errorf("create output dir: %w", err)
		}
	}

	report := &SplitReport{}
	referenced := make(map[string]bool)

	for _, name := range names {
		lowerBSP := "maps/" + name + ".bsp"
		bspData, err := ReadFileFromPk3(fileIndex[lowerBSP], lowerBSP)
		if err != nil {
			report.Failed = append(report.Failed, name+" (read bsp)")
			continue
		}
		bspAssets, err := ParseBSP(bytes.NewReader(bspData), int64(len(bspData)))
		if err != nil {
			report.Failed = append(report.Failed, name+" (parse bsp)")
			continue
		}

		needed := resolveMapNeeds(name, lowerBSP, bspAssets, gm)
		if aas := "maps/" + name + ".aas"; src[aas].pk3 != "" {
			needed[aas] = true
		}

		missing := missingAudio(append(append([]string{}, bspAssets.Sounds...), bspAssets.Music...), fileIndex, stockIndex)

		var keep []string
		var nbytes int64
		for p := range needed {
			referenced[p] = true
			if sc, ok := stock[p]; ok && src[p].crc == sc {
				continue // the client already has it
			}
			keep = append(keep, p)
			nbytes += src[p].bytes
		}
		if len(keep) == 0 {
			continue
		}
		sort.Strings(keep)

		if !opts.DryRun {
			files, err := ExtractFilesFromPk3s(keep, fileIndex)
			if err != nil {
				report.Failed = append(report.Failed, name+" (extract)")
				continue
			}
			outPath := filepath.Join(opts.OutputDir, opts.Prefix+name+".pk3")
			if err := WritePk3(outPath, files); err != nil {
				return nil, fmt.Errorf("write %s: %w", outPath, err)
			}
		}

		report.Maps = append(report.Maps, MapReport{Name: name, FileCount: len(keep), Bytes: nbytes, MissingAudio: missing})
		report.TotalBytes += nbytes
	}

	for path, e := range src {
		if !referenced[path] {
			report.DeadFiles++
			report.DeadBytes += e.bytes
		}
	}
	return report, nil
}

// Refs absent from both the sources and stock — the engine can never load them.
func missingAudio(refs []string, source, stock map[string]string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, ref := range refs {
		if _, ok := resolveAudioPath(ref, source); ok {
			continue
		}
		if _, ok := resolveAudioPath(ref, stock); ok {
			continue
		}
		if !seen[ref] {
			seen[ref] = true
			out = append(out, ref)
		}
	}
	return out
}
