package assets

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ArenaMeta is the parsed shape of one { map ... longname ... } block from a
// Quake III arena definition file (scripts/arenas.txt or scripts/<map>.arena).
type ArenaMeta struct {
	Map       string `json:"-"`                    // lowercase map id; key in the output map, not serialised
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
