package collector

import "testing"

func TestParseTrinityHandshakePlatform(t *testing.T) {
	cases := []struct {
		line string
		want TrinityHandshakeData
	}{
		{
			"2026-10-09T10:00:00 TrinityHandshake: 3 1 v1.1.0 Trinity_Engine_v1.1.0 quest",
			TrinityHandshakeData{ClientNum: 3, Proto: 1, Version: "v1.1.0", Engine: "Trinity_Engine_v1.1.0", Platform: "quest"},
		},
		{
			"2026-10-09T10:00:00 TrinityHandshake: 5 1 v1.1.0 Trinity_Engine_v1.1.0 windows ernie 0123abcd",
			TrinityHandshakeData{ClientNum: 5, Proto: 1, Version: "v1.1.0", Engine: "Trinity_Engine_v1.1.0", Platform: "windows", Username: "ernie", TokenHash: "0123abcd"},
		},
	}
	for _, c := range cases {
		ev, err := ParseLine(c.line)
		if err != nil {
			t.Fatalf("ParseLine(%q): %v", c.line, err)
		}
		if ev.Type != EventTypeTrinityHandshake {
			t.Fatalf("ParseLine(%q): type %q, want %q", c.line, ev.Type, EventTypeTrinityHandshake)
		}
		if got := ev.Data.(TrinityHandshakeData); got != c.want {
			t.Errorf("ParseLine(%q) = %+v, want %+v", c.line, got, c.want)
		}
	}
}
