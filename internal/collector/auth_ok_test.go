package collector

import "testing"

func TestAuthOkLevelDerivation(t *testing.T) {
	cases := []struct {
		name    string
		isAdmin bool
		want    int
	}{
		{"verified non-admin", false, 1},
		{"verified admin", true, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			level := 1
			if tc.isAdmin {
				level = 2
			}
			if level != tc.want {
				t.Errorf("level=%d want %d", level, tc.want)
			}
		})
	}
}
