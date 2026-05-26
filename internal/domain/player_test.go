package domain

import "testing"

func TestCleanQ3DisplayName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Sarge", "Sarge"},
		{"^1Nil^4Class", "^1Nil^4Class"},
		{"^0Hidden^1Visible", "Hidden^1Visible"},
		{"^8Also^1Visible", "Also^1Visible"},
		{"^9Kept", "^9Kept"},
		{"Semi;colon", "Semicolon"},
		{"Back\\slash", "Backslash"},
		{"Qu\"ote", "Quote"},
		{"Ctrl\x01Here", "CtrlHere"},
		{"\x7fDel", "Del"},
		{"  Leading", "Leading"},
		{"Too   Many    Spaces", "Too  Many  Spaces"},
		{"^0^0^0", ""},
		{"", ""},
		{"Trail^", "Trail^"},
		{"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij", "ABCDEFGHIJKLMNOPQRSTUVWXYZabcde"},
		{"^1ABCDEFGHIJKLMNOPQRSTUVWXYZ12345", "^1ABCDEFGHIJKLMNOPQRSTUVWXYZ123"},
	}
	for _, c := range cases {
		if got := CleanQ3DisplayName(c.in); got != c.want {
			t.Errorf("CleanQ3DisplayName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
