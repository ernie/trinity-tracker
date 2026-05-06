package main

import "strings"

// ansiReset clears any active foreground color. Discord's ```ansi
// rendering bleeds across newlines without it.
const ansiReset = "\x1b[0m"

// Q3's canonical 8-color palette is fully saturated RGB (e.g. yellow
// is #FFFF00). Two ANSI flavors render that intent differently:
//
//   - Bright variants (90-97): most modern terminals render these as
//     the saturated, named-color RGB values (xterm: 33m=#cdcd00 dim,
//     93m=#FFFF00 true yellow). Faithful to Q3.
//   - Standard variants (30-37): historically dimmer/themed; many
//     palettes (Solarized, macOS Terminal Pro) render 33m as amber.
//
// Discord's ```ansi``` parser only honors 30-37 — passing 90-97
// strips color entirely. So we keep two tables and pick at the call
// site: terminals use q3ToANSI (bright); the Discord embed renderer
// uses q3ToANSIDiscord (standard, with the known yellow=amber quirk).
//
// The canonical Q3 set has exactly 8 colors — see
// ../trinity-engine/code/qcommon/q_shared.h lines 466-474
// (`#define ColorIndex(c) ( ( (c) - '0' ) & 7 )`). Codes like ^8 and
// ^9 don't exist; we leave them literal rather than reproducing the
// engine's wrap-around (which would render ^9 as red).

// q3ColorANSITerm: bright variants for terminal output. Faithful to
// Q3's saturated palette on any modern terminal.
var q3ColorANSITerm = map[byte]string{
	'0': ansiReset,    // black: rendered as default fg (literal black is invisible on dark themes)
	'1': "\x1b[91m",   // red
	'2': "\x1b[92m",   // green
	'3': "\x1b[93m",   // yellow (true bright yellow, not amber)
	'4': "\x1b[94m",   // blue
	'5': "\x1b[96m",   // cyan
	'6': "\x1b[95m",   // magenta / pink
	'7': "\x1b[97m",   // white
}

// q3ColorANSIDiscord: standard codes only — the bright variants
// don't render in Discord's ansi block.
var q3ColorANSIDiscord = map[byte]string{
	'0': ansiReset,
	'1': "\x1b[31m",
	'2': "\x1b[32m",
	'3': "\x1b[33m", // Discord theme renders this as amber/olive — palette quirk
	'4': "\x1b[34m",
	'5': "\x1b[36m",
	'6': "\x1b[35m",
	'7': "\x1b[37m",
}

// q3ToANSI translates Q3 color codes (^0..^7) inside name into ANSI
// escape sequences appropriate for a modern terminal. Any other
// caret sequence (^^, ^a, ^9, trailing ^) passes through literally —
// that diverges from the engine's permissive parser but avoids
// surprising viewers with implicit colors on non-canonical codes.
// Output ends with a reset whenever any color was emitted.
func q3ToANSI(name string) string {
	return q3Translate(name, q3ColorANSITerm)
}

// q3ToANSIDiscord is q3ToANSI's variant for Discord ```ansi``` blocks,
// which only render the standard ANSI codes (30-37).
func q3ToANSIDiscord(name string) string {
	return q3Translate(name, q3ColorANSIDiscord)
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
