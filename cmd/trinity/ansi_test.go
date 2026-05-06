package main

import "testing"

func TestQ3ToANSI(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		want  string
	}{
		{"plain", "ernie", "ernie"},
		// Bright variants (91-97) — modern terminals render these
		// as the saturated named colors (true yellow not amber).
		{"single_red", "^1ernie", "\x1b[91mernie\x1b[0m"},
		{"multi_color", "^1Red^2Green", "\x1b[91mRed\x1b[92mGreen\x1b[0m"},
		{"black_resets", "^0invisible", "\x1b[0minvisible\x1b[0m"},
		{"white", "^7white", "\x1b[97mwhite\x1b[0m"},
		{"empty", "", ""},
		// ^^ is a literal caret in the engine; we keep it literal too.
		{"double_caret_literal", "abc^^def", "abc^^def"},
		// ^a / ^9 don't exist in the canonical Q3 palette. The engine
		// would render them via (c-'0')&7 wrap-around (^a → red,
		// ^9 → red) — we deliberately leave them literal.
		{"non_digit_passthrough", "^aname", "^aname"},
		{"out_of_range_passthrough", "^9name", "^9name"},
		// A trailing ^ has no following byte to interpret; leave literal.
		{"trailing_caret", "name^", "name^"},
		// Color in the middle of a string still emits a trailing reset.
		{"mid_color_resets", "pre^3yel", "pre\x1b[93myel\x1b[0m"},
		// Plays nicely with clan tags using brackets.
		{"clan_tag", "^5|TR|^6noob", "\x1b[96m|TR|\x1b[95mnoob\x1b[0m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := q3ToANSI(tc.in); got != tc.want {
				t.Errorf("q3ToANSI(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// q3ToANSIDiscord uses the standard codes (30-37) because Discord's
// ansi parser strips the bright variants. Same parsing logic, just
// a different lookup table.
func TestQ3ToANSIDiscord(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"plain", "plain"},
		{"^1ernie", "\x1b[31mernie\x1b[0m"},
		{"^3yellow", "\x1b[33myellow\x1b[0m"},
		{"^5|TR|^6noob", "\x1b[36m|TR|\x1b[35mnoob\x1b[0m"},
		// Same passthrough rules as the terminal variant.
		{"abc^^def", "abc^^def"},
		{"^9not_a_color", "^9not_a_color"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := q3ToANSIDiscord(tc.in); got != tc.want {
				t.Errorf("q3ToANSIDiscord(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
