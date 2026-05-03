package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
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
	entCPMAArmor   = "item_armor_jacket"
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

	HasCPMAArmor bool
}

// mapRow is one entry in the scan results.
type mapRow struct {
	Name    string
	Pk3     string
	Ents    mapEntities
	HasBots bool // a matching .aas file exists in some pk3 (bot navigation data)
}

// mapFilter holds the per-flag filter values parsed from the CLI. Numeric
// filters use 0/MaxInt as no-op sentinels so the keep predicate is just
// arithmetic with no extra "is set" tracking.
type mapFilter struct {
	minDM, maxDM                     int
	minTeamPlayers, maxTeamPlayers   int
	minTeamRespawns, maxTeamRespawns int

	ctf, neutralFlag                bool
	obelisks, neutralObelisk        bool
	taPowerup, taHoldable, taWeapon bool
	cpma                            bool
	bots                            bool
}

// keep ANDs every active filter together. Numeric bounds for both teams must
// each fall within the bound (a CTF map with 2 red + 8 blue spawns is a 2v2
// map for filtering purposes).
func (f mapFilter) keep(r mapRow) bool {
	e := r.Ents
	if e.DM < f.minDM || e.DM > f.maxDM {
		return false
	}
	if e.RedPlayer < f.minTeamPlayers || e.BluePlayer < f.minTeamPlayers {
		return false
	}
	if e.RedPlayer > f.maxTeamPlayers || e.BluePlayer > f.maxTeamPlayers {
		return false
	}
	if e.RedSpawn < f.minTeamRespawns || e.BlueSpawn < f.minTeamRespawns {
		return false
	}
	if e.RedSpawn > f.maxTeamRespawns || e.BlueSpawn > f.maxTeamRespawns {
		return false
	}
	if f.ctf && !(e.HasRedFlag && e.HasBlueFlag) {
		return false
	}
	if f.neutralFlag && !e.HasNeutralFlag {
		return false
	}
	if f.obelisks && !(e.HasRedObelisk && e.HasBlueObelisk) {
		return false
	}
	if f.neutralObelisk && !e.HasNeutObelisk {
		return false
	}
	if f.taPowerup && !e.HasTAPowerup {
		return false
	}
	if f.taHoldable && !e.HasTAHoldable {
		return false
	}
	if f.taWeapon && !e.HasTAWeapon {
		return false
	}
	if f.cpma && !e.HasCPMAArmor {
		return false
	}
	if f.bots && !r.HasBots {
		return false
	}
	return true
}

// labels returns the active filters in CLI form, suitable for echoing back
// in the footer summary. Order matches the help text.
func (f mapFilter) labels() []string {
	var out []string
	if f.minDM > 0 {
		out = append(out, fmt.Sprintf("--min-dm %d", f.minDM))
	}
	if f.maxDM < math.MaxInt {
		out = append(out, fmt.Sprintf("--max-dm %d", f.maxDM))
	}
	if f.minTeamPlayers > 0 {
		out = append(out, fmt.Sprintf("--min-team-players %d", f.minTeamPlayers))
	}
	if f.maxTeamPlayers < math.MaxInt {
		out = append(out, fmt.Sprintf("--max-team-players %d", f.maxTeamPlayers))
	}
	if f.minTeamRespawns > 0 {
		out = append(out, fmt.Sprintf("--min-team-respawns %d", f.minTeamRespawns))
	}
	if f.maxTeamRespawns < math.MaxInt {
		out = append(out, fmt.Sprintf("--max-team-respawns %d", f.maxTeamRespawns))
	}
	if f.ctf {
		out = append(out, "--ctf")
	}
	if f.neutralFlag {
		out = append(out, "--neutral-flag")
	}
	if f.obelisks {
		out = append(out, "--obelisks")
	}
	if f.neutralObelisk {
		out = append(out, "--neutral-obelisk")
	}
	if f.taPowerup {
		out = append(out, "--ta-powerup")
	}
	if f.taHoldable {
		out = append(out, "--ta-holdable")
	}
	if f.taWeapon {
		out = append(out, "--ta-weapon")
	}
	if f.cpma {
		out = append(out, "--cpma")
	}
	if f.bots {
		out = append(out, "--bots")
	}
	return out
}

func cmdMaps(args []string) {
	fs := flag.NewFlagSet("maps", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	namesOnly := fs.Bool("names-only", false, "print only map names, one per line (suitable for piping into rotation files)")

	var f mapFilter
	fs.IntVar(&f.minDM, "min-dm", 0, "require at least N info_player_deathmatch spawns")
	fs.IntVar(&f.maxDM, "max-dm", math.MaxInt, "allow at most N info_player_deathmatch spawns")
	fs.IntVar(&f.minTeamPlayers, "min-team-players", 0, "require at least N initial spawns per team (both teams)")
	fs.IntVar(&f.maxTeamPlayers, "max-team-players", math.MaxInt, "allow at most N initial spawns per team (both teams)")
	fs.IntVar(&f.minTeamRespawns, "min-team-respawns", 0, "require at least N respawns per team (both teams)")
	fs.IntVar(&f.maxTeamRespawns, "max-team-respawns", math.MaxInt, "allow at most N respawns per team (both teams)")
	fs.BoolVar(&f.ctf, "ctf", false, "require both red and blue CTF flag bases")
	fs.BoolVar(&f.neutralFlag, "neutral-flag", false, "require neutral flag (One Flag CTF)")
	fs.BoolVar(&f.obelisks, "obelisks", false, "require both red and blue obelisks (Overload)")
	fs.BoolVar(&f.neutralObelisk, "neutral-obelisk", false, "require neutral obelisk (Harvester)")
	fs.BoolVar(&f.taPowerup, "ta-powerup", false, "require a Team Arena persistent powerup (scout/guard/doubler/ammoregen)")
	fs.BoolVar(&f.taHoldable, "ta-holdable", false, "require a Team Arena holdable (kamikaze/invulnerability)")
	fs.BoolVar(&f.taWeapon, "ta-weapon", false, "require a Team Arena weapon (nailgun/chaingun/proximity launcher)")
	fs.BoolVar(&f.cpma, "cpma", false, "require CPMA green armor (item_armor_jacket)")
	fs.BoolVar(&f.bots, "bots", false, "require bot support (matching .aas navigation file in any pk3)")

	// Cosmetic: hide the MaxInt sentinel in --help; the value still defaults to MaxInt.
	for _, name := range []string{"max-dm", "max-team-players", "max-team-respawns"} {
		fs.Lookup(name).DefValue = "no limit"
	}

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: trinity maps [filters...] [--names-only] [path]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Scans pk3 files for map entities and reports each map's capabilities.")
		fmt.Fprintln(os.Stderr, "Without [path], uses server.quake3_dir from the config (default /usr/lib/quake3).")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Filters AND together. Examples:")
		fmt.Fprintln(os.Stderr, "  trinity maps --ctf --min-team-players 4       playable 4v4 CTF maps")
		fmt.Fprintln(os.Stderr, "  trinity maps --cpma --min-dm 6                CPMA-aware FFA maps")
		fmt.Fprintln(os.Stderr, "  trinity maps --neutral-flag --ta-powerup      1FCTF with TA powerups")
		fmt.Fprintln(os.Stderr, "  trinity maps --bots --min-dm 4                FFA maps that support bots")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Options:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
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

	if *namesOnly {
		streamMapNames(pk3Files, inputPath, f)
		return
	}
	streamMapsTable(pk3Files, inputPath, f)
}

// bspLocation records where to find a .bsp's entity lump.
type bspLocation struct {
	pk3Path    string
	pk3Display string
	bspName    string // path inside the pk3
}

// indexMaps walks every pk3's central directory (no decompression) to find all
// .bsp entries and the matching .aas (bot navigation) files. Returns the bsp
// index keyed by lowercased map basename, the alphabetically sorted list of
// names, and a set of names whose .aas was found in any pk3. Last-loaded pk3
// wins on bsp duplicates, matching the engine's resource-resolution precedence;
// .aas presence is a union across all pk3s because companion bot packs commonly
// ship .aas separately from the .bsp.
func indexMaps(pk3Files []string, basePath string) (map[string]bspLocation, []string, map[string]bool) {
	index := make(map[string]bspLocation)
	aasMaps := make(map[string]bool)
	for _, pk3 := range pk3Files {
		display := pk3DisplayPath(pk3, basePath)
		r, err := zip.OpenReader(pk3)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %s: %v\n", display, err)
			continue
		}
		for _, f := range r.File {
			lower := strings.ToLower(f.Name)
			switch {
			case strings.HasSuffix(lower, ".bsp"):
				name := strings.ToLower(strings.TrimSuffix(filepath.Base(f.Name), filepath.Ext(f.Name)))
				index[name] = bspLocation{pk3Path: pk3, pk3Display: display, bspName: f.Name}
			case strings.HasSuffix(lower, ".aas"):
				name := strings.ToLower(strings.TrimSuffix(filepath.Base(f.Name), filepath.Ext(f.Name)))
				aasMaps[name] = true
			}
		}
		r.Close()
	}
	names := make([]string, 0, len(index))
	for n := range index {
		names = append(names, n)
	}
	sort.Strings(names)
	return index, names, aasMaps
}

// streamFromIndex iterates the sorted name list, parses each map's entities,
// and calls visit. Maintains a 1-entry pk3 cache so adjacent maps in the same
// pk3 (common — e.g. all q3wcp* live in q3wpak1.pk3) skip re-opening. aasMaps
// supplies the bot-support flag for each row.
func streamFromIndex(index map[string]bspLocation, names []string, aasMaps map[string]bool, visit func(mapRow)) int {
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
		visit(mapRow{Name: name, Pk3: loc.pk3Display, Ents: ents, HasBots: aasMaps[name]})
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
		case entCPMAArmor:
			ents.HasCPMAArmor = true
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
		{"CPMA", 6, false, true},
		{"BOTS", 6, false, true},
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
func streamMapsTable(pk3Files []string, basePath string, f mapFilter) {
	printColumnKey(os.Stdout)
	labels := f.labels()
	if len(labels) > 0 {
		fmt.Fprintf(os.Stdout, "\nFilters: %s\n\n", strings.Join(labels, " "))
	} else {
		fmt.Fprintln(os.Stdout)
	}

	index, names, aasMaps := indexMaps(pk3Files, basePath)
	cols := defaultMapsCols()
	cols[0].width = longestName(names) + 2 // +2 for inner padding

	out := os.Stdout
	fmt.Fprintln(out, borderLine(cols, "┌", "┬", "┐"))
	fmt.Fprintln(out, headerRow(cols))
	fmt.Fprintln(out, borderLine(cols, "├", "┼", "┤"))

	matched := 0
	scanned := streamFromIndex(index, names, aasMaps, func(r mapRow) {
		if !f.keep(r) {
			return
		}
		matched++
		fmt.Fprintln(out, dataRow(cols, r))
	})

	fmt.Fprintln(out, borderLine(cols, "└", "┴", "┘"))

	if len(labels) > 0 {
		fmt.Fprintf(out, "\n%d of %d maps matched filters: %s\n", matched, scanned, strings.Join(labels, " "))
	} else {
		fmt.Fprintf(out, "\nScanned %d maps.\n", scanned)
	}
}

// streamMapNames emits just bare map names alphabetically as they're parsed.
func streamMapNames(pk3Files []string, basePath string, f mapFilter) {
	index, names, aasMaps := indexMaps(pk3Files, basePath)
	streamFromIndex(index, names, aasMaps, func(r mapRow) {
		if !f.keep(r) {
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
		padEmoji(e.HasCPMAArmor, cols[13].width),
		padEmoji(r.HasBots, cols[14].width),
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
	fmt.Fprintln(w, "  CPMA   has CPMA green armor (item_armor_jacket)")
	fmt.Fprintln(w, "  BOTS   has matching .aas bot navigation file (in any pk3)")
}
