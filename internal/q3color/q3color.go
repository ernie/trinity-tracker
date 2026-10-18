// Package q3color translates Quake III caret color codes (^0..^7) into
// ANSI escapes (terminal and Discord palettes), strips them, and
// measures ANSI-aware visible width for the CLI, the Discord digest
// and the notify embeds.
package q3color

import "strings"

// ansiReset clears any active foreground color. Discord's ```ansi
// rendering bleeds across newlines without it.
const ansiReset = "\x1b[0m"

// Quake III's eight colors are fully saturated RGB (yellow is
// #FFFF00). The two ANSI flavors render them differently:
//
//   - Bright variants (90-97): most terminals render these as the
//     saturated named colors (xterm: 33m=#cdcd00, 93m=#FFFF00).
//   - Standard variants (30-37): dimmer or themed; many palettes
//     (Solarized, macOS Terminal Pro) render 33m as amber.
//
// Discord's ```ansi``` parser honors only 30-37 and drops color for
// 90-97, so terminals use ToANSI (bright) and the Discord embeds use
// ToANSIDiscord (standard, where yellow shows as amber).
//
// The engine has exactly eight colors (ColorIndex in
// trinity-engine/code/qcommon/q_shared.h masks with & 7); ^8 and ^9
// stay literal here.

var q3ColorANSITerm = map[byte]string{
	'0': ansiReset, // black text vanishes on dark themes, so ^0 uses the default color
	'1': "\x1b[91m",
	'2': "\x1b[92m",
	'3': "\x1b[93m",
	'4': "\x1b[94m",
	'5': "\x1b[96m",
	'6': "\x1b[95m",
	'7': "\x1b[97m",
}

var q3ColorANSIDiscord = map[byte]string{
	'0': ansiReset,
	'1': "\x1b[31m",
	'2': "\x1b[32m",
	'3': "\x1b[33m",
	'4': "\x1b[34m",
	'5': "\x1b[36m",
	'6': "\x1b[35m",
	'7': "\x1b[37m",
}

// ToANSI translates color codes (^0..^7) inside name into ANSI escape
// sequences for a terminal. Any other caret sequence (^^, ^a, ^9, a
// trailing ^) passes through literally. Output ends with a reset
// whenever a color was emitted.
func ToANSI(name string) string {
	return q3Translate(name, q3ColorANSITerm)
}

// ToANSIDiscord is ToANSI's variant for Discord ```ansi``` blocks,
// which only render the standard ANSI codes (30-37).
func ToANSIDiscord(name string) string {
	return q3Translate(name, q3ColorANSIDiscord)
}

// Strip removes all ^N caret-color sequences from name, for Discord
// embed fields that don't render ANSI (titles, descriptions, author
// names).
func Strip(name string) string {
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		if name[i] == '^' && i+1 < len(name) {
			c := name[i+1]
			if c >= '0' && c <= '7' {
				i++
				continue
			}
		}
		b.WriteByte(name[i])
	}
	return b.String()
}

func q3Translate(name string, table map[byte]string) string {
	var b strings.Builder
	var emitted bool
	for i := 0; i < len(name); i++ {
		if name[i] == '^' && i+1 < len(name) {
			if esc, ok := table[name[i+1]]; ok {
				b.WriteString(esc)
				emitted = true
				i++ // consume the digit
				continue
			}
		}
		b.WriteByte(name[i])
	}
	if emitted {
		b.WriteString(ansiReset)
	}
	return b.String()
}
