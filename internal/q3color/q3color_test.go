package q3color

import "testing"

func TestQ3ToANSI(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "alice", "alice"},
		{"single_red", "^1alice", "\x1b[91malice\x1b[0m"},
		{"multi_color", "^1Red^2Green", "\x1b[91mRed\x1b[92mGreen\x1b[0m"},
		{"black_resets", "^0invisible", "\x1b[0minvisible\x1b[0m"},
		{"white", "^7white", "\x1b[97mwhite\x1b[0m"},
		{"empty", "", ""},
		{"double_caret_literal", "abc^^def", "abc^^def"},
		{"non_digit_passthrough", "^aname", "^aname"},
		{"out_of_range_passthrough", "^9name", "^9name"},
		{"trailing_caret", "name^", "name^"},
		{"mid_color_resets", "pre^3yel", "pre\x1b[93myel\x1b[0m"},
		{"clan_tag", "^5|TR|^6noob", "\x1b[96m|TR|\x1b[95mnoob\x1b[0m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ToANSI(tc.in); got != tc.want {
				t.Errorf("ToANSI(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestQ3ToANSIDiscord(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"plain", "plain"},
		{"^1alice", "\x1b[31malice\x1b[0m"},
		{"^3yellow", "\x1b[33myellow\x1b[0m"},
		{"^5|TR|^6noob", "\x1b[36m|TR|\x1b[35mnoob\x1b[0m"},
		{"abc^^def", "abc^^def"},
		{"^9not_a_color", "^9not_a_color"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := ToANSIDiscord(tc.in); got != tc.want {
				t.Errorf("ToANSIDiscord(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStripQ3Colors(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"plain", "plain"},
		{"^1alice", "alice"},
		{"^1Red^2Green^7White", "RedGreenWhite"},
		{"abc^^def", "abc^^def"},
		{"^9not_a_color", "^9not_a_color"},
		{"trailing^", "trailing^"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := Strip(tc.in); got != tc.want {
				t.Errorf("Strip(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
