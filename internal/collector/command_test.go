package collector

import "testing"

func TestParseCommand(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantCmd string
		wantArg string
	}{
		{"plain no-arg", "claim", "claim", ""},
		{"plain with arg", "link 123456", "link", "123456"},
		// The mod appends a trailing ^7 (S_COLOR_WHITE) to the logged command.
		{"trailing color, no-arg", "claim^7", "claim", ""},
		{"trailing color, help", "help^7", "help", ""},
		{"trailing color on arg", "link 123456^7", "link", "123456"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, args := parseCommand(tc.raw)
			if cmd != tc.wantCmd || args != tc.wantArg {
				t.Errorf("parseCommand(%q) = (%q, %q), want (%q, %q)",
					tc.raw, cmd, args, tc.wantCmd, tc.wantArg)
			}
		})
	}
}
