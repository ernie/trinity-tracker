package collector

import "testing"

func TestParseAssistTypes(t *testing.T) {
	cases := []struct {
		line       string
		clientID   int
		team       int
		assistType string
		name       string
	}{
		{"2026-06-12T10:00:00 Assist: 6 2 return: Visor", 6, 2, "return", "Visor"},
		{"2026-06-12T10:00:00 Assist: 2 1 frag: WarLokk", 2, 1, "frag", "WarLokk"},
		{"2026-06-12T10:00:00 Assist: 8 2 skull: ^1Nil^4Class", 8, 2, "skull", "^1Nil^4Class"},
		{"2026-06-12T10:00:00 Assist: 3 1 obelisk: Grunt", 3, 1, "obelisk", "Grunt"},
		{"2026-06-12T10:00:00 Assist: 4 2 carry: Punisher", 4, 2, "carry", "Punisher"},
		{"2026-06-12T10:00:00 Assist: 5 1 damage: Morgan", 5, 1, "damage", "Morgan"},
	}
	for _, c := range cases {
		ev, err := ParseLine(c.line)
		if err != nil {
			t.Fatalf("ParseLine(%q): %v", c.line, err)
		}
		if ev.Type != EventTypeAssist {
			t.Fatalf("ParseLine(%q): type %q, want %q", c.line, ev.Type, EventTypeAssist)
		}
		d := ev.Data.(AssistData)
		if d.ClientID != c.clientID || d.Team != c.team || d.AssistType != c.assistType || d.Name != c.name {
			t.Fatalf("ParseLine(%q): got %+v", c.line, d)
		}
	}
}

// The mod logs assists only via "Assist:" lines, never as an Award.
func TestParseAwardRejectsAssistToken(t *testing.T) {
	ev, err := ParseLine("2026-06-12T10:00:00 Award: 5 assist: Visor")
	if err == nil && ev != nil && ev.Type == EventTypeAward {
		t.Fatalf("Award assist line parsed as award: %+v", ev.Data)
	}
}

func TestParseAssistRejectsNonLowercaseType(t *testing.T) {
	for _, line := range []string{
		"2026-06-12T10:00:00 Assist: 6 2 RETURN: Visor",
		"2026-06-12T10:00:00 Assist: 6 2 : Visor",
	} {
		ev, err := ParseLine(line)
		if err == nil && ev != nil && ev.Type == EventTypeAssist {
			t.Fatalf("ParseLine(%q) matched as assist; want non-assist", line)
		}
	}
}
