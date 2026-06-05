package q3color

import "testing"

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
			if got := VisibleWidth(tc.in); got != tc.want {
				t.Errorf("VisibleWidth(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
