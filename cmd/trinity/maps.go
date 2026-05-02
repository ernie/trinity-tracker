package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	flag "github.com/spf13/pflag"
)

// Q3 BSP entity classnames the scanner cares about.
const (
	entDM          = "info_player_deathmatch"
	entRedPlayer   = "team_ctf_redplayer"
	entRedSpawn    = "team_ctf_redspawn"
	entBluePlayer  = "team_ctf_blueplayer"
	entBlueSpawn   = "team_ctf_bluespawn"
	entRedFlag     = "team_ctf_redflag"
	entBlueFlag    = "team_ctf_blueflag"
	entNeutralFlag = "team_ctf_neutralflag"
	entRedObelisk  = "team_redobelisk"
	entBlueObelisk = "team_blueobelisk"
	entNeutObelisk = "team_neutralobelisk"
)

// Team Arena items. Holdable kamikaze/invulnerability are the TA additions
// (holdable_teleporter and holdable_medkit ship in baseq3).
var (
	taPowerups  = map[string]bool{"item_scout": true, "item_guard": true, "item_doubler": true, "item_ammoregen": true}
	taHoldables = map[string]bool{"holdable_kamikaze": true, "holdable_invulnerability": true}
	taWeapons   = map[string]bool{"weapon_nailgun": true, "weapon_chaingun": true, "weapon_prox_launcher": true}
)

// mapEntities is what we extract per .bsp.
type mapEntities struct {
	DM         int
	RedPlayer  int
	RedSpawn   int
	BluePlayer int
	BlueSpawn  int

	HasRedFlag     bool
	HasBlueFlag    bool
	HasNeutralFlag bool
	HasRedObelisk  bool
	HasBlueObelisk bool
	HasNeutObelisk bool

	HasTAPowerup  bool
	HasTAHoldable bool
	HasTAWeapon   bool
}

// hasTeamSpawns reports whether both teams have at least one spawn (initial or respawn).
func (m mapEntities) hasTeamSpawns() bool {
	return (m.RedPlayer+m.RedSpawn) > 0 && (m.BluePlayer+m.BlueSpawn) > 0
}

// hasFullTAOutfit means the map has both a persistent TA powerup and a TA weapon
// placed in the level. Holdables are informational (not all TA maps include them).
func (m mapEntities) hasFullTAOutfit() bool {
	return m.HasTAPowerup && m.HasTAWeapon
}

// mapRow is one entry in the scan results.
type mapRow struct {
	Name string
	Pk3  string
	Ents mapEntities
}

// mapMode describes a Trinity-supported mode and its requirements.
type mapMode struct {
	canonical string   // canonical name (matches parseGametype's preferred token)
	aliases   []string // accepted CLI tokens
	desc      string
	suitable  func(mapEntities) bool
}

var supportedModes = []mapMode{
	{
		canonical: "ffa",
		aliases:   []string{"ffa"},
		desc:      "Free-For-All",
		suitable:  func(e mapEntities) bool { return e.DM > 0 },
	},
	{
		canonical: "1v1",
		aliases:   []string{"1v1", "tournament"},
		desc:      "Tournament (1v1)",
		suitable:  func(e mapEntities) bool { return e.DM > 0 },
	},
	{
		canonical: "tdm",
		aliases:   []string{"tdm"},
		desc:      "Team Deathmatch",
		suitable:  func(e mapEntities) bool { return e.DM > 0 },
	},
	{
		canonical: "tdm-ta",
		aliases:   []string{"tdm-ta"},
		desc:      "Team Deathmatch (Team Arena)",
		suitable:  func(e mapEntities) bool { return e.DM > 0 && e.hasFullTAOutfit() },
	},
	{
		canonical: "ctf",
		aliases:   []string{"ctf"},
		desc:      "Capture The Flag",
		suitable: func(e mapEntities) bool {
			return e.hasTeamSpawns() && e.HasRedFlag && e.HasBlueFlag
		},
	},
	{
		canonical: "ctf-ta",
		aliases:   []string{"ctf-ta"},
		desc:      "Capture The Flag (Team Arena)",
		suitable: func(e mapEntities) bool {
			return e.hasTeamSpawns() && e.HasRedFlag && e.HasBlueFlag && e.hasFullTAOutfit()
		},
	},
	{
		canonical: "1fctf",
		aliases:   []string{"1fctf", "oneflag"},
		desc:      "One Flag CTF",
		suitable: func(e mapEntities) bool {
			return e.hasTeamSpawns() &&
				e.HasRedFlag && e.HasBlueFlag && e.HasNeutralFlag &&
				e.hasFullTAOutfit()
		},
	},
	{
		canonical: "overload",
		aliases:   []string{"overload"},
		desc:      "Overload",
		suitable: func(e mapEntities) bool {
			return e.hasTeamSpawns() && e.HasRedObelisk && e.HasBlueObelisk && e.hasFullTAOutfit()
		},
	},
	{
		canonical: "harvester",
		aliases:   []string{"harvester"},
		desc:      "Harvester",
		suitable: func(e mapEntities) bool {
			return e.hasTeamSpawns() &&
				e.HasRedObelisk && e.HasBlueObelisk && e.HasNeutObelisk &&
				e.hasFullTAOutfit()
		},
	},
}

// findMode resolves a CLI token to a registered mode.
func findMode(name string) (mapMode, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, m := range supportedModes {
		for _, a := range m.aliases {
			if a == name {
				return m, true
			}
		}
	}
	return mapMode{}, false
}

func cmdMaps(args []string) {
	fs := flag.NewFlagSet("maps", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	mode := fs.String("mode", "", "list only maps suitable for this mode (ffa, 1v1, tdm, tdm-ta, ctf, ctf-ta, 1fctf, overload, harvester)")
	namesOnly := fs.Bool("names-only", false, "print only map names, one per line (suitable for piping into rotation files)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: trinity maps [--mode <mode>] [--names-only] [path]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Scans pk3 files for map entities and reports which game modes each map supports.")
		fmt.Fprintln(os.Stderr, "Without [path], uses server.quake3_dir from the config (default /usr/lib/quake3).")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Modes accepted by --mode:")
		for _, m := range supportedModes {
			fmt.Fprintf(os.Stderr, "  %-10s %s\n", m.canonical, m.desc)
		}
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Options:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	// Validate --mode before touching config/disk so bad input fails fast.
	var filterMode mapMode
	var haveFilter bool
	if *mode != "" {
		m, ok := findMode(*mode)
		if !ok {
			names := make([]string, len(supportedModes))
			for i, m := range supportedModes {
				names[i] = m.canonical
			}
			fmt.Fprintf(os.Stderr, "Error: unknown mode %q. Try one of: %s\n", *mode, strings.Join(names, ", "))
			os.Exit(2)
		}
		filterMode = m
		haveFilter = true
	}

	// Resolve input dir: positional arg wins; else config; else default.
	inputPath := ""
	if extras := fs.Args(); len(extras) > 0 {
		inputPath = extras[0]
	}
	if inputPath == "" {
		if cfg := loadCLIConfigFromFlags(*configPath, ""); cfg != nil {
			inputPath = cfg.Server.Quake3Dir
		}
		if inputPath == "" {
			inputPath = "/usr/lib/quake3"
		}
	}

	pk3Files := collectPk3FilesOrdered(inputPath)
	if len(pk3Files) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no pk3 files found in %s\n", inputPath)
		os.Exit(1)
	}

	keep := func(mapEntities) bool { return true }
	if haveFilter {
		keep = filterMode.suitable
	}

	if *namesOnly {
		streamMapNames(pk3Files, inputPath, keep)
		return
	}
	streamMapsTable(pk3Files, inputPath, keep, haveFilter, filterMode)
}

// bspLocation records where to find a .bsp's entity lump.
type bspLocation struct {
	pk3Path    string
	pk3Display string
	bspName    string // path inside the pk3
}

// indexMaps walks every pk3's central directory (no decompression) to find all
// .bsp entries. Returns the index keyed by lowercased map basename and the
// alphabetically sorted list of names. Last-loaded pk3 wins on duplicates,
// matching the engine's resource-resolution precedence.
func indexMaps(pk3Files []string, basePath string) (map[string]bspLocation, []string) {
	index := make(map[string]bspLocation)
	for _, pk3 := range pk3Files {
		display := pk3DisplayPath(pk3, basePath)
		r, err := zip.OpenReader(pk3)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %s: %v\n", display, err)
			continue
		}
		for _, f := range r.File {
			if !strings.HasSuffix(strings.ToLower(f.Name), ".bsp") {
				continue
			}
			name := strings.ToLower(strings.TrimSuffix(filepath.Base(f.Name), filepath.Ext(f.Name)))
			index[name] = bspLocation{pk3Path: pk3, pk3Display: display, bspName: f.Name}
		}
		r.Close()
	}
	names := make([]string, 0, len(index))
	for n := range index {
		names = append(names, n)
	}
	sort.Strings(names)
	return index, names
}

// streamFromIndex iterates the sorted name list, parses each map's entities,
// and calls visit. Maintains a 1-entry pk3 cache so adjacent maps in the same
// pk3 (common — e.g. all q3wcp* live in q3wpak1.pk3) skip re-opening.
func streamFromIndex(index map[string]bspLocation, names []string, visit func(mapRow)) int {
	var (
		cachePath  string
		cacheR     *zip.ReadCloser
		cacheFiles map[string]*zip.File
	)
	defer func() {
		if cacheR != nil {
			cacheR.Close()
		}
	}()

	scanned := 0
	for _, name := range names {
		loc := index[name]
		if loc.pk3Path != cachePath {
			if cacheR != nil {
				cacheR.Close()
				cacheR = nil
				cacheFiles = nil
				cachePath = ""
			}
			r, err := zip.OpenReader(loc.pk3Path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: %s: %v\n", loc.pk3Display, err)
				continue
			}
			cacheR = r
			cachePath = loc.pk3Path
			cacheFiles = make(map[string]*zip.File, len(r.File))
			for _, f := range r.File {
				cacheFiles[f.Name] = f
			}
		}
		zf, ok := cacheFiles[loc.bspName]
		if !ok {
			fmt.Fprintf(os.Stderr, "  Warning: %s:%s: bsp entry vanished\n", loc.pk3Display, loc.bspName)
			continue
		}
		ents, err := readMapEntities(zf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %s:%s: %v\n", loc.pk3Display, loc.bspName, err)
			continue
		}
		scanned++
		visit(mapRow{Name: name, Pk3: loc.pk3Display, Ents: ents})
	}
	return scanned
}

// readMapEntities decompresses one .bsp from a pk3 and parses its entity lump.
// Q3 BSP layout: magic "IBSP" (4) + version (4) + 17 lumps × {offset, length}.
// Lump 0 is the entity string.
func readMapEntities(f *zip.File) (mapEntities, error) {
	rc, err := f.Open()
	if err != nil {
		return mapEntities{}, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return mapEntities{}, err
	}
	if len(data) < 8+17*8 {
		return mapEntities{}, fmt.Errorf("file too small to be a Q3 BSP")
	}
	if !bytes.Equal(data[:4], []byte("IBSP")) {
		return mapEntities{}, fmt.Errorf("not a Q3 BSP (missing IBSP magic)")
	}

	off := int(int32(binary.LittleEndian.Uint32(data[8:12])))
	length := int(int32(binary.LittleEndian.Uint32(data[12:16])))
	if off < 0 || length <= 0 || off+length > len(data) {
		return mapEntities{}, fmt.Errorf("entity lump out of range")
	}

	return parseEntityClasses(data[off : off+length]), nil
}

// parseEntityClasses pulls the "classname" value out of every entity block in
// the entity lump and tallies the ones we care about. The lump is plain ASCII;
// each entity is a brace-delimited block of `"key" "value"` pairs.
func parseEntityClasses(lump []byte) mapEntities {
	var ents mapEntities
	for _, cn := range entityClassnames(lump) {
		switch cn {
		case entDM:
			ents.DM++
		case entRedPlayer:
			ents.RedPlayer++
		case entRedSpawn:
			ents.RedSpawn++
		case entBluePlayer:
			ents.BluePlayer++
		case entBlueSpawn:
			ents.BlueSpawn++
		case entRedFlag:
			ents.HasRedFlag = true
		case entBlueFlag:
			ents.HasBlueFlag = true
		case entNeutralFlag:
			ents.HasNeutralFlag = true
		case entRedObelisk:
			ents.HasRedObelisk = true
		case entBlueObelisk:
			ents.HasBlueObelisk = true
		case entNeutObelisk:
			ents.HasNeutObelisk = true
		default:
			if taPowerups[cn] {
				ents.HasTAPowerup = true
			}
			if taHoldables[cn] {
				ents.HasTAHoldable = true
			}
			if taWeapons[cn] {
				ents.HasTAWeapon = true
			}
		}
	}
	return ents
}

// entityClassnames yields lowercase classname values from a Q3 entity lump.
// We do a minimal walk rather than a full key/value parser: find every
// `"classname"` token and read the next quoted string.
func entityClassnames(lump []byte) []string {
	const key = `"classname"`
	var out []string
	i := 0
	for {
		idx := bytes.Index(lump[i:], []byte(key))
		if idx < 0 {
			return out
		}
		j := i + idx + len(key)
		// Skip whitespace until the opening quote of the value.
		for j < len(lump) && lump[j] != '"' {
			j++
		}
		if j >= len(lump) {
			return out
		}
		j++ // past opening quote
		start := j
		for j < len(lump) && lump[j] != '"' {
			j++
		}
		if j > len(lump) {
			return out
		}
		out = append(out, strings.ToLower(string(lump[start:j])))
		i = j + 1
	}
}

// Box-drawn table layout. Widths are fixed so rows can be streamed without
// buffering. Numeric columns size for up to 5 digits; flag columns assume
// each emoji renders as 2 cells (true on every modern terminal).
type col struct {
	header  string
	width   int  // inner width (between vertical bars), excluding bar but including padding
	numeric bool // numeric columns are right-aligned counts; non-numeric flags use ✅/❌
	emoji   bool // true for the flag columns (FLAG..WPN)
}

// defaultMapsCols returns a fresh copy of the column layout. The MAP column's
// width is sized by the caller after a pre-pass over pk3 directories.
func defaultMapsCols() []col {
	return []col{
		{"MAP", 5, false, false},
		{"DM", 5, true, false},
		{"R-PLR", 7, true, false},
		{"R-SPN", 7, true, false},
		{"B-PLR", 7, true, false},
		{"B-SPN", 7, true, false},
		{"FLAG", 6, false, true},
		{"1F", 4, false, true},
		{"OBEL", 6, false, true},
		{"HRV", 5, false, true},
		{"POW", 5, false, true},
		{"HLD", 5, false, true},
		{"WPN", 5, false, true},
	}
}

// longestName returns the longest string in names, with a floor of len("MAP")
// so the column is at least wide enough for its header.
func longestName(names []string) int {
	max := len("MAP")
	for _, n := range names {
		if len(n) > max {
			max = len(n)
		}
	}
	return max
}

const (
	emojiYes = "✅"
	emojiNo  = "❌"
)

// streamMapsTable scans pk3s alphabetically and emits each row to stdout as
// it's parsed, wrapped in a Unicode box-drawn table. With filter, non-matching
// rows are silently dropped.
func streamMapsTable(pk3Files []string, basePath string, keep func(mapEntities) bool, filtered bool, m mapMode) {
	printColumnKey(os.Stdout)
	if filtered {
		fmt.Fprintf(os.Stdout, "\nMaps suitable for %s (%s):\n\n", m.canonical, m.desc)
	} else {
		fmt.Fprintln(os.Stdout)
	}

	index, names := indexMaps(pk3Files, basePath)
	cols := defaultMapsCols()
	cols[0].width = longestName(names) + 2 // +2 for inner padding

	out := os.Stdout
	fmt.Fprintln(out, borderLine(cols, "┌", "┬", "┐"))
	fmt.Fprintln(out, headerRow(cols))
	fmt.Fprintln(out, borderLine(cols, "├", "┼", "┤"))

	matched := 0
	scanned := streamFromIndex(index, names, func(r mapRow) {
		if !keep(r.Ents) {
			return
		}
		matched++
		fmt.Fprintln(out, dataRow(cols, r))
	})

	fmt.Fprintln(out, borderLine(cols, "└", "┴", "┘"))

	if filtered {
		fmt.Fprintf(out, "\n%d maps suitable for %s out of %d scanned.\n", matched, m.canonical, scanned)
	} else {
		fmt.Fprintf(out, "\nScanned %d maps.\n", scanned)
	}
}

// streamMapNames emits just bare map names alphabetically as they're parsed.
func streamMapNames(pk3Files []string, basePath string, keep func(mapEntities) bool) {
	index, names := indexMaps(pk3Files, basePath)
	streamFromIndex(index, names, func(r mapRow) {
		if !keep(r.Ents) {
			return
		}
		fmt.Println(r.Name)
	})
}

func borderLine(cols []col, left, mid, right string) string {
	var b strings.Builder
	b.WriteString(left)
	for i, c := range cols {
		b.WriteString(strings.Repeat("─", c.width))
		if i < len(cols)-1 {
			b.WriteString(mid)
		}
	}
	b.WriteString(right)
	return b.String()
}

func headerRow(cols []col) string {
	var b strings.Builder
	b.WriteString("│")
	for _, c := range cols {
		b.WriteString(padCenter(c.header, c.width))
		b.WriteString("│")
	}
	return b.String()
}

func dataRow(cols []col, r mapRow) string {
	e := r.Ents
	cells := []string{
		padLeft(r.Name, cols[0].width),
		padRightInt(e.DM, cols[1].width),
		padRightInt(e.RedPlayer, cols[2].width),
		padRightInt(e.RedSpawn, cols[3].width),
		padRightInt(e.BluePlayer, cols[4].width),
		padRightInt(e.BlueSpawn, cols[5].width),
		padEmoji(e.HasRedFlag && e.HasBlueFlag, cols[6].width),
		padEmoji(e.HasNeutralFlag, cols[7].width),
		padEmoji(e.HasRedObelisk && e.HasBlueObelisk, cols[8].width),
		padEmoji(e.HasNeutObelisk, cols[9].width),
		padEmoji(e.HasTAPowerup, cols[10].width),
		padEmoji(e.HasTAHoldable, cols[11].width),
		padEmoji(e.HasTAWeapon, cols[12].width),
	}
	return "│" + strings.Join(cells, "│") + "│"
}

// padLeft left-aligns s in a cell of the given width with 1-char inner padding.
func padLeft(s string, width int) string {
	body := " " + s
	if len(body) > width {
		body = body[:width]
	}
	return body + strings.Repeat(" ", width-len(body))
}

// padRightInt right-aligns the integer in a cell with 1-char inner padding.
func padRightInt(n, width int) string {
	s := fmt.Sprintf("%d", n)
	pad := width - 1 - len(s)
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + s + " "
}

// padCenter horizontally centers ASCII text in a cell of the given width.
func padCenter(s string, width int) string {
	if len(s) >= width {
		return s
	}
	left := (width - len(s)) / 2
	right := width - len(s) - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

// padEmoji centers a 2-cell-wide emoji glyph in a cell of the given width.
// Most modern terminals render emoji presentation-form codepoints as 2 cells.
func padEmoji(present bool, width int) string {
	sym := emojiNo
	if present {
		sym = emojiYes
	}
	const visual = 2
	extra := width - visual
	if extra < 0 {
		return sym
	}
	left := extra / 2
	right := extra - left
	return strings.Repeat(" ", left) + sym + strings.Repeat(" ", right)
}

// printColumnKey explains the table columns so the report is self-describing.
func printColumnKey(w io.Writer) {
	fmt.Fprintln(w, "Columns:")
	fmt.Fprintln(w, "  MAP    map name (.bsp filename without extension)")
	fmt.Fprintln(w, "  DM     count of info_player_deathmatch (FFA/1v1/TDM spawns)")
	fmt.Fprintln(w, "  R-PLR  count of team_CTF_redplayer (red team initial spawns)")
	fmt.Fprintln(w, "  R-SPN  count of team_CTF_redspawn  (red team respawns)")
	fmt.Fprintln(w, "  B-PLR  count of team_CTF_blueplayer (blue team initial spawns)")
	fmt.Fprintln(w, "  B-SPN  count of team_CTF_bluespawn  (blue team respawns)")
	fmt.Fprintln(w, "  FLAG   has both red and blue CTF flag bases")
	fmt.Fprintln(w, "  1F     has neutral flag (One Flag CTF)")
	fmt.Fprintln(w, "  OBEL   has both red and blue obelisks (Overload)")
	fmt.Fprintln(w, "  HRV    has neutral obelisk (Harvester skull spawner)")
	fmt.Fprintln(w, "  POW    has at least one Team Arena persistent powerup (scout/guard/doubler/ammoregen)")
	fmt.Fprintln(w, "  HLD    has at least one Team Arena holdable (kamikaze/invulnerability)")
	fmt.Fprintln(w, "  WPN    has at least one Team Arena weapon (nailgun/chaingun/proximity launcher)")
}
