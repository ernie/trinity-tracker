package storage

import "testing"

func TestCanonicalizeDisplayName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Foo", "foo"},
		{"^1Foo", "foo"},
		{"^1Foo^7Bar", "foobar"},
		{"[VR] Foo", "foo"},
		{"^7[VR] ^1Foo", "foo"},
		{"Foo  Bar", "foo bar"},
		{"  Foo  Bar  ", "foo bar"},
		{"Foo Bar [VR]", "foo bar"},
		{"^1Foo^7   Bar^1", "foo bar"},
		{"", ""},
		{"[VR]", ""},
		// Case-insensitive: different casing, same canonical form
		{"Foo", "foo"},
		{"FOO", "foo"},
	}
	for _, c := range cases {
		if got := CanonicalizeDisplayName(c.in); got != c.want {
			t.Errorf("CanonicalizeDisplayName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
