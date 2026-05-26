package storage

import "testing"

func TestCanonicalizeDisplayName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Foo", "Foo"},
		{"^1Foo", "Foo"},
		{"^1Foo^7Bar", "FooBar"},
		{"[VR] Foo", "Foo"},
		{"^7[VR] ^1Foo", "Foo"},
		{"Foo  Bar", "Foo Bar"},
		{"  Foo  Bar  ", "Foo Bar"},
		{"Foo Bar [VR]", "Foo Bar"},
		{"^1Foo^7   Bar^1", "Foo Bar"},
		{"", ""},
		{"[VR]", ""},
		// Case is preserved (NOT lowercased)
		{"Foo", "Foo"},
		{"FOO", "FOO"},
	}
	for _, c := range cases {
		if got := CanonicalizeDisplayName(c.in); got != c.want {
			t.Errorf("CanonicalizeDisplayName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
