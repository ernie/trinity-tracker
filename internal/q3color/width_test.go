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
		{"héllo", 5},
		{"hi\x1b[31m", 2},
		{"🥽", 2},
		{"🖥️", 2},
		{"a🥽b", 4},
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
