package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/ernie/trinity-tracker/internal/discord"
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
// contain ANSI escape sequences; discord.AnsiVisibleWidth measures
// only printable runes so layout stays correct.
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
		visibleWidths[i] = discord.AnsiVisibleWidth(c.header)
		for _, cell := range c.cells {
			if cw := discord.AnsiVisibleWidth(cell); cw > visibleWidths[i] {
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
			padding := visibleWidths[i] - discord.AnsiVisibleWidth(v)
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
