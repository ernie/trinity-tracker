package discord

import "unicode/utf8"

// AnsiVisibleWidth returns the printable cell width of s — what a
// monospace font actually paints — accounting for:
//
//   - ANSI CSI escapes (ESC [ ... letter): zero width
//   - SMP emoji (U+1F000 and above): 2 cells
//   - variation selectors / ZWJ: 0 width (modifiers only)
//   - everything else: 1 cell
//
// We deliberately don't widen BMP symbol chars (U+2600-27FF, which
// includes Dingbats like ✓ and Misc Symbols like ★). Most monospace
// fonts render those at 1 cell despite their visual emoji-ness. Not
// a full East Asian Width implementation — trinity output doesn't
// render CJK.
func AnsiVisibleWidth(s string) int {
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
			// Variation selector 16 and zero-width joiner contribute no
			// width of their own.
		case r >= 0x1F000:
			width += 2
		default:
			width++
		}
		i += size
	}
	return width
}
