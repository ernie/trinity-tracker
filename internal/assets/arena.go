package assets

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

// ArenaMeta is the parsed shape of one { map ... longname ... } block from a
// Quake III arena definition file (scripts/arenas.txt or scripts/<map>.arena).
type ArenaMeta struct {
	Map       string `json:"-"` // lowercase map id; key in the output map, not serialised
	LongName  string `json:"longname,omitempty"`
	Type      string `json:"type,omitempty"`
	FragLimit int    `json:"fraglimit,omitempty"`
	Author    string `json:"author,omitempty"`
}

// ParseArenaText parses the brace-block format used by Q3 arena files.
// Tokens are simple: a `{` opens a block, a `}` closes it, and within a block
// each line is `key   "value"` (or `key value` for numeric values).
// Whitespace-tolerant; unknown keys are silently dropped.
func ParseArenaText(r io.Reader) ([]ArenaMeta, error) {
	var out []ArenaMeta
	var current *ArenaMeta

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "{"):
			current = &ArenaMeta{}
		case strings.HasPrefix(line, "}"):
			if current != nil && current.Map != "" {
				out = append(out, *current)
			}
			current = nil
		default:
			if current == nil {
				continue
			}
			key, value := splitKeyValue(line)
			switch key {
			case "map":
				current.Map = strings.ToLower(value)
			case "longname":
				current.LongName = value
			case "type":
				current.Type = value
			case "fraglimit":
				if n, err := strconv.Atoi(value); err == nil {
					current.FragLimit = n
				}
			case "author":
				current.Author = value
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan arena text: %w", err)
	}
	return out, nil
}

// splitKeyValue parses a `key "value"` or `key value` line into its parts.
// Quoted values may contain spaces; unquoted values are taken whole.
func splitKeyValue(line string) (key, value string) {
	idx := strings.IndexAny(line, " \t")
	if idx < 0 {
		return line, ""
	}
	key = line[:idx]
	rest := strings.TrimSpace(line[idx:])
	if strings.HasPrefix(rest, `"`) {
		end := strings.Index(rest[1:], `"`)
		if end >= 0 {
			return key, rest[1 : 1+end]
		}
	}
	return key, rest
}

// ExtractArenas walks every pk3 in the given list, parses every scripts/*.arena
// and scripts/arenas.txt file it finds, and returns a map keyed by lowercase
// map id. Later pk3s override earlier ones — matches how the engine itself
// resolves overlapping defs.
func ExtractArenas(pk3s []string) (map[string]ArenaMeta, error) {
	out := make(map[string]ArenaMeta)
	for _, pk3Path := range pk3s {
		if err := IteratePk3(pk3Path, func(name string, open func() (io.ReadCloser, error)) error {
			if !isArenaFile(strings.ToLower(name)) {
				return nil
			}
			rc, err := open()
			if err != nil {
				return fmt.Errorf("open %s in %s: %w", name, pk3Path, err)
			}
			defer rc.Close()
			entries, err := ParseArenaText(rc)
			if err != nil {
				return fmt.Errorf("parse %s in %s: %w", name, pk3Path, err)
			}
			for _, e := range entries {
				if e.Map == "" {
					continue
				}
				out[e.Map] = e
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// isArenaFile returns true for paths under scripts/ that end in .arena
// (per-map files) or are exactly scripts/arenas.txt / scripts/missionpack.arena
// (the bundled master files).
func isArenaFile(p string) bool {
	dir, name := path.Split(p)
	if dir != "scripts/" {
		return false
	}
	return strings.HasSuffix(name, ".arena") || name == "arenas.txt"
}
