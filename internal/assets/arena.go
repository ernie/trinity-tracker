package assets

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
)

// ArenaMeta is one maps.json entry, keyed by lowercase map id.
type ArenaMeta struct {
	Map      string `json:"-"`
	LongName string `json:"longname,omitempty"`
}

// MapNameSources accumulates map display-name evidence across pk3s. Worldspawn
// messages and .arena longnames are kept apart until Resolve, because a map's
// .arena file may ride in a different pk3 than its .bsp, so neither source's
// pk3 load order can decide precedence between them.
type MapNameSources struct {
	bspMessages map[string]string   // map id -> worldspawn "message"
	arenaNames  map[string]string   // map id -> .arena longname
	mapIDs      map[string]struct{} // every map id that has a .bsp
}

func NewMapNameSources() *MapNameSources {
	return &MapNameSources{
		bspMessages: make(map[string]string),
		arenaNames:  make(map[string]string),
		mapIDs:      make(map[string]struct{}),
	}
}

// Collect reads one pk3 into the source set. Call across pk3s in load order:
// later pk3s overwrite earlier entries within each source, matching how the
// engine resolves overlapping files.
func (s *MapNameSources) Collect(pk3Path string) error {
	return IteratePk3(pk3Path, func(name string, open func() (io.ReadCloser, error)) error {
		lower := strings.ToLower(name)
		switch {
		case strings.HasPrefix(lower, "maps/") && strings.HasSuffix(lower, ".bsp"):
			rc, err := open()
			if err != nil {
				return fmt.Errorf("open %s in %s: %w", name, pk3Path, err)
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				return fmt.Errorf("read %s in %s: %w", name, pk3Path, err)
			}
			mapID := strings.TrimSuffix(strings.TrimPrefix(lower, "maps/"), ".bsp")
			s.mapIDs[mapID] = struct{}{}
			msg, err := BSPWorldspawnMessage(bytes.NewReader(data), int64(len(data)))
			if err != nil || msg == "" {
				return nil
			}
			s.bspMessages[mapID] = msg
		case strings.HasPrefix(lower, "scripts/") && strings.HasSuffix(lower, ".arena"):
			rc, err := open()
			if err != nil {
				return fmt.Errorf("open %s in %s: %w", name, pk3Path, err)
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				return fmt.Errorf("read %s in %s: %w", name, pk3Path, err)
			}
			for id, longName := range parseArenaLongNames(data) {
				s.arenaNames[id] = longName
			}
		}
		return nil
	})
}

// Resolve returns the id→meta map for every installed map, named by its
// worldspawn message when present (the authoritative in-game loading name),
// otherwise by its .arena longname. Maps that have a .bsp but no name from
// either source, and .arena entries for maps that are not installed, are
// omitted; see Unnamed.
func (s *MapNameSources) Resolve() map[string]ArenaMeta {
	out := make(map[string]ArenaMeta, len(s.mapIDs))
	for id := range s.mapIDs {
		switch {
		case s.bspMessages[id] != "":
			out[id] = ArenaMeta{Map: id, LongName: s.bspMessages[id]}
		case s.arenaNames[id] != "":
			out[id] = ArenaMeta{Map: id, LongName: s.arenaNames[id]}
		}
	}
	return out
}

// ArenaFilled lists installed maps (sorted) that had no worldspawn message and
// were named from a .arena file.
func (s *MapNameSources) ArenaFilled() []string {
	var ids []string
	for id := range s.mapIDs {
		if s.bspMessages[id] == "" && s.arenaNames[id] != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// Unnamed lists installed maps (sorted) that have no longname from any source.
func (s *MapNameSources) Unnamed() []string {
	var ids []string
	for id := range s.mapIDs {
		if s.bspMessages[id] == "" && s.arenaNames[id] == "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// ExtractMapLongNames resolves every installed map's display name across pk3s.
func ExtractMapLongNames(pk3s []string) (map[string]ArenaMeta, error) {
	s := NewMapNameSources()
	for _, pk3Path := range pk3s {
		if err := s.Collect(pk3Path); err != nil {
			return nil, err
		}
	}
	return s.Resolve(), nil
}

// parseArenaLongNames maps lowercase map id → longname from a Quake 3 .arena
// file: a flat stream of { map "id" longname "Name" ... } blocks.
func parseArenaLongNames(content []byte) map[string]string {
	out := make(map[string]string)
	toks := tokenizeArena(content)
	for i := 0; i < len(toks); {
		if toks[i] != "{" {
			i++
			continue
		}
		i++ // consume '{'
		var mapID, longName string
		for i < len(toks) && toks[i] != "}" {
			key := strings.ToLower(toks[i])
			i++
			if i >= len(toks) || toks[i] == "}" || toks[i] == "{" {
				break
			}
			val := toks[i]
			i++
			switch key {
			case "map":
				mapID = strings.ToLower(val)
			case "longname":
				longName = val
			}
		}
		if mapID != "" && longName != "" {
			out[mapID] = longName
		}
	}
	return out
}

// tokenizeArena splits .arena text into tokens: quoted strings (unquoted),
// the braces { and }, and barewords. // line comments are dropped.
func tokenizeArena(content []byte) []string {
	s := string(content)
	var toks []string
	for i := 0; i < len(s); {
		switch c := s[i]; {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == '"':
			j := i + 1
			for j < len(s) && s[j] != '"' {
				j++
			}
			toks = append(toks, s[i+1:j])
			i = j + 1
		case c == '{' || c == '}':
			toks = append(toks, string(c))
			i++
		case c == '/' && i+1 < len(s) && s[i+1] == '/':
			for i < len(s) && s[i] != '\n' {
				i++
			}
		default:
			j := i
			for j < len(s) && !strings.ContainsRune(" \t\r\n\"{}", rune(s[j])) {
				j++
			}
			toks = append(toks, s[i:j])
			i = j
		}
	}
	return toks
}
