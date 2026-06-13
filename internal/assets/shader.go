package assets

import (
	"bufio"
	"io"
	"strings"
)

// ShaderTextureDef represents a parsed shader definition and its texture dependencies.
type ShaderTextureDef struct {
	Name     string
	Textures []string
}

// ParseShaderTextures parses a .shader text file and extracts shader definitions
// with their texture dependencies.
func ParseShaderTextures(r io.Reader) ([]ShaderTextureDef, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for large shader files

	var shaders []ShaderTextureDef
	var current *ShaderTextureDef
	depth := 0
	inBlockComment := false

	for scanner.Scan() {
		line := scanner.Text()

		// Handle block comments
		if inBlockComment {
			if idx := strings.Index(line, "*/"); idx >= 0 {
				line = line[idx+2:]
				inBlockComment = false
			} else {
				continue
			}
		}

		// Process comments: find whichever comment marker comes first
		for {
			slashSlash := strings.Index(line, "//")
			slashStar := strings.Index(line, "/*")

			if slashStar >= 0 && (slashSlash < 0 || slashStar < slashSlash) {
				// /* comes first — handle block comment
				endIdx := strings.Index(line[slashStar+2:], "*/")
				if endIdx >= 0 {
					line = line[:slashStar] + line[slashStar+2+endIdx+2:]
					continue // re-check for more comments
				} else {
					line = line[:slashStar]
					inBlockComment = true
					break
				}
			} else if slashSlash >= 0 {
				// // comes first — strip rest of line
				line = line[:slashSlash]
				break
			} else {
				break
			}
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Process braces and content, handling compact formatting where
		// braces share a line with directives (e.g. "{ map foo.tga")
		for line != "" {
			if line[0] == '{' {
				depth++
				line = strings.TrimSpace(line[1:])
				continue
			}
			if line[0] == '}' {
				depth--
				if depth == 0 && current != nil {
					shaders = append(shaders, *current)
					current = nil
				}
				line = strings.TrimSpace(line[1:])
				continue
			}

			// Extract content up to the next brace (or end of line)
			var content string
			if idx := strings.IndexAny(line, "{}"); idx >= 0 {
				content = strings.TrimSpace(line[:idx])
				line = line[idx:] // leave brace for next iteration
			} else {
				content = line
				line = ""
			}

			if content == "" {
				continue
			}

			if depth == 0 {
				// Shader name
				current = &ShaderTextureDef{Name: content}
				continue
			}

			if current == nil {
				continue
			}

			// Parse directives inside shader (depth >= 1)
			tokens := tokenizeLine(content)
			if len(tokens) == 0 {
				continue
			}

			directive := strings.ToLower(tokens[0])
			switch directive {
			case "map", "clampmap", "diffusemap", "normalmap", "specularmap":
				if len(tokens) >= 2 {
					path := tokens[1]
					if !strings.HasPrefix(path, "$") {
						current.Textures = append(current.Textures, path)
					}
				}
			case "animmap":
				// animMap <freq> <path1> <path2> ...
				if len(tokens) >= 3 {
					for _, path := range tokens[2:] {
						if !strings.HasPrefix(path, "$") {
							current.Textures = append(current.Textures, path)
						}
					}
				}
			case "videomap":
				// Pathless names get a video/ prefix at load time
				// (cl_cin.c CIN_PlayCinematic)
				if len(tokens) >= 2 {
					path := tokens[1]
					if !strings.Contains(path, "/") && !strings.Contains(path, "\\") {
						path = "video/" + path
					}
					current.Textures = append(current.Textures, path)
				}
			case "skyparms":
				// skyparms <farbox> - -
				if len(tokens) >= 2 && tokens[1] != "-" {
					base := tokens[1]
					for _, suffix := range []string{"_rt", "_lf", "_bk", "_ft", "_up", "_dn"} {
						current.Textures = append(current.Textures, base+suffix)
					}
				}
			}
		}
	}

	return shaders, scanner.Err()
}

// tokenizeLine splits a shader line into whitespace-separated tokens.
func tokenizeLine(line string) []string {
	return strings.Fields(line)
}
