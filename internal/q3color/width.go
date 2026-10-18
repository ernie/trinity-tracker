package q3color

import "unicode/utf8"

// VisibleWidth returns the cell width a monospace font paints for s:
//
//   - ANSI CSI escapes (ESC [ ... letter): zero width
//   - SMP emoji (U+1F000 and above): 2 cells
//   - variation selector 16 and ZWJ: zero width
//   - everything else: 1 cell, including BMP symbols like ✓ and ★,
//     which most monospace fonts draw single-width
func VisibleWidth(s string) int {
	width := 0
	i := 0
	for i < len(s) {
		// CSI introducer is ESC (\x1b) followed by '['. Skip until
		// the terminator (any byte in the @-~ range).
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) {
				b := s[j]
				j++
				if b >= '@' && b <= '~' {
					break
				}
			}
			i = j
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if size == 0 {
			size = 1
		}
		switch {
		case r == 0xFE0F || r == 0x200D:
		case r >= 0x1F000:
			width += 2
		default:
			width++
		}
		i += size
	}
	return width
}
