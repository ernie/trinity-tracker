package assets

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// TcMod is one texcoord modifier (scroll|scale|rotate) with its numeric args.
type TcMod struct {
	Type string    `json:"type"`
	Args []float32 `json:"args"`
}

// ShaderStage is one render pass of a shader.
type ShaderStage struct {
	Maps      []string `json:"maps"`
	AnimFreq  float32  `json:"animFreq,omitempty"`
	Blend     string   `json:"blend"`
	AlphaFunc string   `json:"alphaFunc,omitempty"`
	TcGen     string   `json:"tcGen,omitempty"`
	TcMod     []TcMod  `json:"tcMod,omitempty"`
	RgbGen    string   `json:"rgbGen"`
	Clamp     bool     `json:"clamp,omitempty"`
}

// ShaderDef is a parsed shader: surface-level cull/deform plus its stages.
type ShaderDef struct {
	DoubleSided bool
	Deform      string
	Stages      []ShaderStage
}

func atof(s string) float32 {
	f, _ := strconv.ParseFloat(s, 32)
	return float32(f)
}

func mapBlend(toks []string) string {
	if len(toks) == 1 {
		switch strings.ToLower(toks[0]) {
		case "add":
			return "add"
		case "blend":
			return "blend"
		case "filter":
			return "filter"
		}
		return "opaque"
	}
	if len(toks) >= 2 {
		src, dst := strings.ToUpper(toks[0]), strings.ToUpper(toks[1])
		switch {
		case src == "GL_ONE" && dst == "GL_ZERO":
			return "opaque"
		case src == "GL_ONE" && dst == "GL_ONE":
			return "add"
		case src == "GL_SRC_ALPHA" && dst == "GL_ONE_MINUS_SRC_ALPHA":
			return "blend"
		case src == "GL_DST_COLOR" || (src == "GL_ZERO" && dst == "GL_SRC_COLOR"):
			return "filter"
		}
	}
	return "blend"
}

// ParseShaderScript parses a Q3 .shader file into shader definitions keyed by
// lowercased shader name.
func ParseShaderScript(r io.Reader) (map[string]ShaderDef, error) {
	defs := map[string]ShaderDef{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)

	var name string
	var def ShaderDef
	var stage *ShaderStage
	depth := 0
	inShader := false

	flushStage := func() {
		if stage != nil {
			if stage.Blend == "" {
				stage.Blend = "opaque"
			}
			if stage.RgbGen == "" {
				stage.RgbGen = "lightingDiffuse"
			}
			def.Stages = append(def.Stages, *stage)
			stage = nil
		}
	}

	var pending []string
	nextLine := func() (string, bool) {
		if len(pending) > 0 {
			l := pending[0]
			pending = pending[1:]
			return l, true
		}
		if !sc.Scan() {
			return "", false
		}
		line := strings.TrimSpace(sc.Text())
		if i := strings.Index(line, "//"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		// Braces are standalone tokens to the engine even when sharing a line
		// with a directive; split them onto their own logical lines.
		for _, b := range []string{"{", "}"} {
			if line != b && strings.Contains(line, b) {
				line = strings.ReplaceAll(line, b, "\n"+b+"\n")
			}
		}
		if strings.Contains(line, "\n") {
			parts := strings.Split(line, "\n")
			line = strings.TrimSpace(parts[0])
			for _, p := range parts[1:] {
				if p = strings.TrimSpace(p); p != "" {
					pending = append(pending, p)
				}
			}
		}
		return line, true
	}

	for {
		line, ok := nextLine()
		if !ok {
			break
		}
		if line == "" {
			continue
		}
		if line == "{" {
			depth++
			if depth == 2 {
				stage = &ShaderStage{}
			}
			continue
		}
		if line == "}" {
			if depth == 2 {
				flushStage()
			} else if depth == 1 {
				defs[strings.ToLower(name)] = def
				inShader = false
			}
			depth--
			continue
		}
		fields := strings.Fields(line)
		key := strings.ToLower(fields[0])

		if depth == 0 {
			name = fields[0]
			def = ShaderDef{}
			inShader = true
			continue
		}
		if !inShader {
			continue
		}
		if depth == 1 {
			if key == "cull" && len(fields) > 1 {
				c := strings.ToLower(fields[1])
				def.DoubleSided = c == "disable" || c == "none" || c == "twosided"
			}
			if key == "deformvertexes" && len(fields) > 1 {
				if d := strings.ToLower(fields[1]); d == "autosprite" || d == "autosprite2" {
					def.Deform = d
				}
			}
			continue
		}
		if stage == nil {
			continue
		}
		switch key {
		case "map":
			if len(fields) > 1 {
				stage.Maps = []string{fields[1]}
			}
		case "clampmap":
			if len(fields) > 1 {
				stage.Maps = []string{fields[1]}
				stage.Clamp = true
			}
		case "animmap":
			if len(fields) > 2 {
				stage.AnimFreq = atof(fields[1])
				stage.Maps = append([]string{}, fields[2:]...)
			}
		case "blendfunc":
			stage.Blend = mapBlend(fields[1:])
		case "alphafunc":
			if len(fields) > 1 {
				stage.AlphaFunc = strings.ToLower(fields[1])
			}
		case "tcgen":
			if len(fields) > 1 && strings.ToLower(fields[1]) == "environment" {
				stage.TcGen = "environment"
			}
		case "tcmod":
			if len(fields) > 1 {
				t := strings.ToLower(fields[1])
				if t == "scroll" || t == "scale" || t == "rotate" {
					var args []float32
					for _, a := range fields[2:] {
						args = append(args, atof(a))
					}
					stage.TcMod = append(stage.TcMod, TcMod{Type: t, Args: args})
				}
			}
		case "rgbgen":
			if len(fields) > 1 {
				if strings.ToLower(fields[1]) == "identity" {
					stage.RgbGen = "identity"
				} else {
					stage.RgbGen = "lightingDiffuse"
				}
			}
		}
	}
	return defs, sc.Err()
}

// MergeShaderDefs copies src defs into dst; later definitions win, matching
// the engine's last-loaded-shader precedence.
func MergeShaderDefs(dst, src map[string]ShaderDef) {
	for k, v := range src {
		dst[k] = v
	}
}
