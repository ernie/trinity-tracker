package assets

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
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
	bytes int64
}

// stockSet is what every client already has: the official paks and the
// Trinity paks of the baseline games.
type stockSet struct {
	files map[string]string // lowered path → pak
	paks  []string          // load order
}

// Guard against bundling id Software's copyrighted assets.
func stockBaseline(quake3Dir string, games []string) (*stockSet, error) {
	allPaks := CollectGamePk3s(quake3Dir)
	stock := &stockSet{files: make(map[string]string)}
	official := false
	for _, game := range games {
		for _, p := range allPaks[game] {
			if IsOfficialPak(p) {
				official = true
			} else if !IsTrinityPak(p) {
				continue
			}
			stock.paks = append(stock.paks, p)
		}
	}
	if !official {
		return nil, fmt.Errorf("no official pak[0-9].pk3 found under %s for games %v, so not running", quake3Dir, games)
	}

	for _, p := range stock.paks {
		r, err := zip.OpenReader(p)
		if err != nil {
			return nil, fmt.Errorf("open stock pak %s: %w", p, err)
		}
		for _, f := range r.File {
			if !f.FileInfo().IsDir() {
				stock.files[strings.ToLower(f.Name)] = p
			}
		}
		r.Close()
	}
	return stock, nil
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
			index[strings.ToLower(f.Name)] = srcEntry{p, int64(f.CompressedSize64)}
		}
		r.Close()
	}
	return index, nil
}

// SplitMapPack writes one standalone pk3 per map. Each carries its shader
// definitions in a generated script, so every pk3 holds the same definition
// for a name however many of them a client installs.
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

	srcIndex := make(map[string]string, len(src))
	for path, e := range src {
		srcIndex[path] = e.pk3
	}
	files := make(map[string]string, len(src)+len(stock.files))
	for path, pk3 := range srcIndex {
		files[path] = pk3
	}
	for path, pak := range stock.files {
		files[path] = pak
	}

	stockShaders, err := BuildShaderIndex(stock.paks)
	if err != nil {
		return nil, fmt.Errorf("index stock shaders: %w", err)
	}
	srcShaders, err := BuildShaderIndex(opts.Sources)
	if err != nil {
		return nil, fmt.Errorf("index source shaders: %w", err)
	}
	shaders := make(map[string]ShaderEntry, len(srcShaders)+len(stockShaders))
	for name, e := range srcShaders {
		shaders[name] = e
	}
	for name, e := range stockShaders {
		shaders[name] = e
	}
	resolver := mapResolver{files: files, shaders: shaders}

	stockPak := make(map[string]bool, len(stock.paks))
	for _, p := range stock.paks {
		stockPak[p] = true
	}
	fromSource := func(e ShaderEntry) bool { return !stockPak[e.Pak] }

	arenas, err := sourceArenas(opts.Sources)
	if err != nil {
		return nil, err
	}

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
		bspData, err := ReadFileFromPk3(srcIndex[lowerBSP], lowerBSP)
		if err != nil {
			report.Failed = append(report.Failed, name+" (read bsp)")
			continue
		}
		bspAssets, err := ParseBSP(bytes.NewReader(bspData), int64(len(bspData)))
		if err != nil {
			report.Failed = append(report.Failed, name+" (parse bsp)")
			continue
		}

		needed, used := resolveMapNeeds(name, lowerBSP, bspAssets, resolver)
		if aas := "maps/" + name + ".aas"; src[aas].pk3 != "" {
			needed[aas] = true
		}
		arenaPath := "scripts/" + name + ".arena"
		delete(needed, arenaPath)

		missing := missingAudio(append(append([]string{}, bspAssets.Sounds...), bspAssets.Music...), srcIndex, stock.files)

		var keep []string
		var nbytes int64
		for p := range needed {
			referenced[p] = true
			if _, ok := stock.files[p]; ok {
				continue // the client already has it
			}
			keep = append(keep, p)
			nbytes += src[p].bytes
		}
		sort.Strings(keep)

		generated := make(map[string][]byte)
		if script := generatedShaderScript(used, fromSource); script != "" {
			generated["scripts/mapsplit_"+name+".shader"] = []byte(script)
			for _, e := range used {
				if fromSource(e) {
					referenced[e.File] = true
				}
			}
		}
		if a, ok := arenas[name]; ok {
			generated[arenaPath] = []byte(a.text + "\n")
			referenced[a.file] = true
		}
		for _, data := range generated {
			nbytes += int64(len(data))
		}

		if len(keep) == 0 && len(generated) == 0 {
			continue
		}

		if !opts.DryRun {
			out, err := ExtractFilesFromPk3s(keep, srcIndex)
			if err != nil {
				report.Failed = append(report.Failed, name+" (extract)")
				continue
			}
			for path, data := range generated {
				out[path] = data
			}
			outPath := filepath.Join(opts.OutputDir, opts.Prefix+name+".pk3")
			if err := WritePk3(outPath, out); err != nil {
				return nil, fmt.Errorf("write %s: %w", outPath, err)
			}
		}

		report.Maps = append(report.Maps, MapReport{Name: name, FileCount: len(keep) + len(generated), Bytes: nbytes, MissingAudio: missing})
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

type arenaEntry struct {
	text string
	file string
}

// sourceArenas maps each map id to its { ... } entry from the sources'
// .arena files and arenas.txt, later sources winning.
func sourceArenas(sources []string) (map[string]arenaEntry, error) {
	out := make(map[string]arenaEntry)
	for _, p := range sources {
		err := IteratePk3(p, func(name string, open func() (io.ReadCloser, error)) error {
			lower := strings.ToLower(name)
			if !strings.HasPrefix(lower, "scripts/") || !(strings.HasSuffix(lower, ".arena") || lower == "scripts/arenas.txt") {
				return nil
			}
			rc, err := open()
			if err != nil {
				return nil
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil
			}
			for _, block := range arenaBlocks(string(data)) {
				if id := arenaMapID(block); id != "" {
					out[id] = arenaEntry{text: block, file: lower}
				}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("read arenas %s: %w", p, err)
		}
	}
	return out, nil
}

// arenaBlocks returns each top-level { ... } entry verbatim.
func arenaBlocks(s string) []string {
	var blocks []string
	start := -1
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"':
			if j := strings.IndexByte(s[i+1:], '"'); j >= 0 {
				i += j + 1
			} else {
				i = len(s)
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '/':
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '{' && start < 0:
			start = i
		case c == '}' && start >= 0:
			blocks = append(blocks, s[start:i+1])
			start = -1
		}
	}
	return blocks
}

func arenaMapID(block string) string {
	toks := tokenizeArena([]byte(block))
	for i := 0; i+1 < len(toks); i++ {
		if strings.EqualFold(toks[i], "map") {
			return strings.ToLower(toks[i+1])
		}
	}
	return ""
}

// missingAudio lists refs in neither the sources nor stock, which the
// engine can never load.
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
