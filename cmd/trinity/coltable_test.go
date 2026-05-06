package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestAnsiVisibleWidth(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"hello", 5},
		{"\x1b[31mhello\x1b[0m", 5},
		{"\x1b[1m\x1b[31mhello\x1b[0m", 5},
		// Multi-byte UTF-8 should count 1 per visible rune
		{"héllo", 5},
		// Trailing escape (incomplete) — pragmatically treat the
		// remaining bytes as zero-width since they wouldn't print
		// anything visible anyway. This branch keeps the function
		// total: it never panics on truncated input.
		{"hi\x1b[31m", 2},
		// SMP emoji render at 2 cells. Variation selector U+FE0F adds
		// no width of its own — the desktop emoji "🖥️" is one
		// 2-cell glyph plus a zero-width VS, total 2.
		{"🥽", 2},
		{"🖥️", 2},
		{"a🥽b", 4},
		// BMP Dingbats like ✓ stay at 1 cell — most monospace fonts
		// render them single-width despite their emoji-ish look.
		{"✓", 1},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := ansiVisibleWidth(tc.in); got != tc.want {
				t.Errorf("ansiVisibleWidth(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// The whole reason this package exists: a row mixing colored and
// plain cells must align identically to a fully-plain layout. We
// strip the escapes from the rendered output and check column
// positions are stable.
func TestRenderTable_AlignsThroughANSI(t *testing.T) {
	withColor(t, true)
	cols := []column{
		{header: "RANK", align: alignRight, cells: []string{"1", "2"}},
		{header: "PLAYER", cells: []string{
			"\x1b[31mernie\x1b[0m",
			"rocketeer",
		}},
		{header: "FRAGS", align: alignRight, cells: []string{"412", "318"}},
	}
	var buf bytes.Buffer
	renderTable(&buf, cols)
	got := stripANSI(buf.String())

	// PLAYER column width is 9 (max of header=6, ernie=5, rocketeer=9).
	// Header "PLAYER" gets 3 chars of left-align padding inside its
	// 9-wide cell; rocketeer fills the cell exactly. The separator
	// row is 9 dashes wide — that's the alignment proof: the colored
	// "ernie" cell didn't fool the width math.
	wantLines := []string{
		"RANK  PLAYER     FRAGS",
		"----  ---------  -----",
		"   1  ernie        412",
		"   2  rocketeer    318",
	}
	gotLines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(gotLines) != len(wantLines) {
		t.Fatalf("line count: got %d, want %d\n%s", len(gotLines), len(wantLines), got)
	}
	for i, want := range wantLines {
		if gotLines[i] != want {
			t.Errorf("line %d:\n  got  %q\n  want %q", i, gotLines[i], want)
		}
	}
}

func TestRenderTable_EmitsEscapesWhenEnabled(t *testing.T) {
	withColor(t, true)
	var buf bytes.Buffer
	renderTable(&buf, []column{{header: "X", cells: []string{"a"}}})
	out := buf.String()
	if !strings.Contains(out, "\x1b[1m") {
		t.Errorf("expected bold header escape; got %q", out)
	}
	if !strings.Contains(out, "\x1b[2m") {
		t.Errorf("expected dim separator escape; got %q", out)
	}
}

// When color is off, the renderer must produce no escape sequences —
// otherwise piped output (NO_COLOR=1, --color=never) would still
// carry junk for grep/awk to deal with.
func TestRenderTable_NoEscapesWhenDisabled(t *testing.T) {
	withColor(t, false)
	var buf bytes.Buffer
	renderTable(&buf, []column{{header: "X", cells: []string{"a"}}})
	if strings.ContainsRune(buf.String(), 0x1b) {
		t.Errorf("expected no ESC bytes when disabled; got %q", buf.String())
	}
}

func TestRenderTable_EmptyHandled(t *testing.T) {
	var buf bytes.Buffer
	renderTable(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("empty table should emit nothing; got %q", buf.String())
	}
}

// stripANSI removes CSI escape sequences from s so tests can assert
// on visible layout regardless of color state.
func stripANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) {
				c := s[j]
				j++
				if c >= '@' && c <= '~' {
					break
				}
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
