package assets

import (
	"io"
	"path/filepath"
	"sort"
	"strings"
)

// ShaderEntry is the definition the engine uses for one shader name.
type ShaderEntry struct {
	Textures []string `json:"textures"`
	File     string   `json:"file"`
	Pak      string   `json:"pak"`
	Text     string   `json:"text"`
}

// BuildShaderIndex resolves every shader name as ScanAndLoadShaderFiles does
// for pk3s given in load order: only the highest-priority copy of each script
// is read, the first script listed wins, and within a script the last
// definition wins.
func BuildShaderIndex(pk3s []string) (map[string]ShaderEntry, error) {
	index := make(map[string]ShaderEntry)
	seen := make(map[string]bool)
	for i := len(pk3s) - 1; i >= 0; i-- {
		pk3 := pk3s[i]
		err := IteratePk3(pk3, func(name string, open func() (io.ReadCloser, error)) error {
			lower := strings.ToLower(name)
			if !strings.HasPrefix(lower, "scripts/") || !strings.HasSuffix(lower, ".shader") || seen[lower] {
				return nil
			}
			seen[lower] = true
			rc, err := open()
			if err != nil {
				return nil
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil
			}
			script := make(map[string]ShaderEntry)
			for _, b := range splitShaderBlocks(string(data)) {
				var textures []string
				if defs, _ := ParseShaderTextures(strings.NewReader(b.text)); len(defs) > 0 {
					textures = defs[0].Textures
				}
				script[strings.ToLower(b.name)] = ShaderEntry{Textures: textures, File: lower, Pak: pk3, Text: b.text}
			}
			for k, v := range script {
				if _, ok := index[k]; !ok {
					index[k] = v
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return index, nil
}

type shaderBlock struct {
	name string
	text string
}

// splitShaderBlocks cuts script text into top-level "name { ... }" blocks,
// tokenizing like the renderer's COM_ParseExt and SkipBracedSection.
func splitShaderBlocks(s string) []shaderBlock {
	pos := 0
	next := func() (tok string, start, end int, ok bool) {
		for pos < len(s) {
			c := s[pos]
			switch {
			case c <= ' ':
				pos++
			case c == '/' && pos+1 < len(s) && s[pos+1] == '/':
				for pos < len(s) && s[pos] != '\n' {
					pos++
				}
			case c == '/' && pos+1 < len(s) && s[pos+1] == '*':
				if end := strings.Index(s[pos+2:], "*/"); end >= 0 {
					pos += end + 4
				} else {
					pos = len(s)
				}
			case c == '"':
				start := pos
				if end := strings.IndexByte(s[pos+1:], '"'); end >= 0 {
					pos += end + 2
					return s[start+1 : pos-1], start, pos, true
				}
				pos = len(s)
				return s[start+1:], start, pos, true
			default:
				start := pos
				for pos < len(s) && s[pos] > ' ' {
					pos++
				}
				return s[start:pos], start, pos, true
			}
		}
		return "", pos, pos, false
	}

	var blocks []shaderBlock
	for {
		name, start, _, ok := next()
		if !ok {
			return blocks
		}
		tok, _, end, ok := next()
		if !ok {
			return blocks
		}
		if tok != "{" {
			continue
		}
		for depth := 1; depth > 0; {
			tok, _, end, ok = next()
			if !ok {
				return blocks
			}
			switch tok {
			case "{":
				depth++
			case "}":
				depth--
			}
		}
		blocks = append(blocks, shaderBlock{name: name, text: s[start:end]})
	}
}

func generatedShaderScript(used map[string]ShaderEntry, keep func(ShaderEntry) bool) string {
	var names []string
	for name, e := range used {
		if e.Text != "" && keep(e) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		b.WriteString(used[name].Text)
		b.WriteString("\n\n")
	}
	return b.String()
}

// isClientPak reports whether every client already loads pak: an official
// pak or a Trinity pak.
func isClientPak(pak string) bool {
	base := filepath.Base(pak)
	return IsOfficialPak(base) || IsTrinityPak(base)
}
