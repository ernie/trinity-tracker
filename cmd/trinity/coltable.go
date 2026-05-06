package main

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// colAlign is right-padded by default; numeric/right-aligned columns
// pass alignRight. We don't bother with center-align — nothing here
// needs it.
type colAlign int

const (
	alignLeft  colAlign = 0
	alignRight colAlign = 1
)

// column is one column's header + cells + alignment. Cells may
// contain ANSI escape sequences; ansiVisibleWidth measures only
// printable runes so layout stays correct.
type column struct {
	header string
	cells  []string
	align  colAlign
}

// renderTable writes a tabular layout to w. Headers are rendered
// bold, the separator row is rendered dim — both via the helpers
// in color.go, so they no-op when color is disabled.
//
// Why not text/tabwriter: tabwriter measures byte width, so any cell
// containing ANSI escapes mis-aligns. This renderer strips ANSI when
// computing column widths and pads after. ~30 lines of code, no
// external deps.
func renderTable(w io.Writer, cols []column) {
	if len(cols) == 0 {
		return
	}
	visibleWidths := make([]int, len(cols))
	nRows := 0
	for i, c := range cols {
		visibleWidths[i] = ansiVisibleWidth(c.header)
		for _, cell := range c.cells {
			if cw := ansiVisibleWidth(cell); cw > visibleWidths[i] {
				visibleWidths[i] = cw
			}
		}
		if len(c.cells) > nRows {
			nRows = len(c.cells)
		}
	}

	// Two-space gutter between columns. The last column gets no
	// trailing gutter — left-align doesn't need it, and right-align
	// already pads on the left.
	emit := func(values []string) {
		for i, v := range values {
			padding := visibleWidths[i] - ansiVisibleWidth(v)
			if padding < 0 {
				padding = 0
			}
			pad := strings.Repeat(" ", padding)
			switch cols[i].align {
			case alignRight:
				fmt.Fprint(w, pad+v)
			default:
				fmt.Fprint(w, v+pad)
			}
			if i < len(values)-1 {
				fmt.Fprint(w, "  ")
			}
		}
		fmt.Fprintln(w)
	}

	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = bold(c.header)
	}
	emit(headers)

	sep := make([]string, len(cols))
	for i := range cols {
		sep[i] = dim(strings.Repeat("-", visibleWidths[i]))
	}
	emit(sep)

	for r := 0; r < nRows; r++ {
		row := make([]string, len(cols))
		for i, c := range cols {
			if r < len(c.cells) {
				row[i] = c.cells[r]
			}
		}
		emit(row)
	}
}

// ansiVisibleWidth returns the printable cell width of s — what a
// monospace font actually paints — accounting for:
//
//   - ANSI CSI escapes (ESC [ ... letter): zero width
//   - SMP emoji (U+1F000 and above): 2 cells
//   - variation selectors / ZWJ: 0 width (modifiers only)
//   - everything else: 1 cell
//
// We deliberately don't widen BMP symbol chars (U+2600-27FF, which
// includes Dingbats like ✓ and Misc Symbols like ★). Most monospace
// fonts render those at 1 cell despite their visual emoji-ness.
// Not a full East Asian Width implementation — trinity output
// doesn't render CJK.
func ansiVisibleWidth(s string) int {
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
			// Variation selector 16 (turns `🖥` → `🖥️` colored emoji)
			// and zero-width joiner contribute no width of their own.
		case r >= 0x1F000:
			// Supplementary Multilingual Plane emoji block.
			width += 2
		default:
			width++
		}
		i += size
	}
	return width
}
