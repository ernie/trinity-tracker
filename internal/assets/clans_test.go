package assets

import "testing"

func TestClanKey(t *testing.T) {
	for name, want := range map[string]string{
		"The Flock":      "theflock",
		"Team Beef":      "teambeef",
		"Crusaders":      "crusaders",
		"  The  Fallen ": "thefallen",
	} {
		if got := ClanKey(name); got != want {
			t.Errorf("ClanKey(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestClanIconPath(t *testing.T) {
	for _, c := range []struct {
		path, key, team string
		ok              bool
	}{
		{"team_icon/TheFlock_red.tga", "theflock", "red", true},
		{"TEAM_ICON/pagans_BLUE.TGA", "pagans", "blue", true},
		{"team_icon/pagans_red.jpg", "", "", false},
		{"team_icon/pagans.tga", "", "", false},
		{"icons/pagans_red.tga", "", "", false},
		{"team_icon/sub/pagans_red.tga", "", "", false},
	} {
		key, team, ok := ClanIconPath(c.path)
		if key != c.key || team != c.team || ok != c.ok {
			t.Errorf("ClanIconPath(%q) = (%q, %q, %v), want (%q, %q, %v)", c.path, key, team, ok, c.key, c.team, c.ok)
		}
	}
}
