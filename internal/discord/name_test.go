package discord

import "testing"

func TestStripVRTag(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"[VR] alice", "alice"},
		{"[VR]alice", "alice"},
		{"^7[VR] alice", "alice"},
		{"^1^7[VR] alice", "alice"},
		{"alice [VR]", "alice"},
		{"alice[VR]", "alice"},
		{"alice [VR]^7", "alice"},
		{"alice", "alice"},
		{"al[VR]ice", "al[VR]ice"}, // middle: leave alone
		{"[VR]", ""},
		{"[vr] alice", "alice"}, // case-insensitive
	}
	for _, c := range cases {
		if got := StripVRTag(c.in); got != c.want {
			t.Errorf("StripVRTag(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDisplayName(t *testing.T) {
	// Conditional: only strip when isVR is true.
	if got := DisplayName("[VR] alice", true); got != "alice" {
		t.Errorf("isVR=true should strip: got %q", got)
	}
	if got := DisplayName("[VR] alice", false); got != "[VR] alice" {
		t.Errorf("isVR=false should preserve: got %q", got)
	}
	// Non-VR-tagged name unaffected either way.
	if got := DisplayName("alice", true); got != "alice" {
		t.Errorf("plain name with isVR=true: got %q", got)
	}
}
