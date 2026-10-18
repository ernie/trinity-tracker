package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/ernie/trinity-tracker/internal/q3color"
)

type colAlign int

const (
	alignLeft  colAlign = 0
	alignRight colAlign = 1
)

// column cells may contain ANSI escape sequences.
type column struct {
	header string
	cells  []string
	align  colAlign
}

// renderTable measures visible width, so cells with ANSI escapes stay
// aligned.
func renderTable(w io.Writer, cols []column) {
	if len(cols) == 0 {
		return
	}
	visibleWidths := make([]int, len(cols))
	nRows := 0
	for i, c := range cols {
		visibleWidths[i] = q3color.VisibleWidth(c.header)
		for _, cell := range c.cells {
			if cw := q3color.VisibleWidth(cell); cw > visibleWidths[i] {
				visibleWidths[i] = cw
			}
		}
		if len(c.cells) > nRows {
			nRows = len(c.cells)
		}
	}

	emit := func(values []string) {
		for i, v := range values {
			padding := visibleWidths[i] - q3color.VisibleWidth(v)
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
