package assets

import (
	"path"
	"strings"
)

// ClanKey names a clan's logo files the way the mod's team icon shaders do.
func ClanKey(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), ""))
}

// ClanIconPath parses team_icon/<clan>_<team>.tga, the pre-colored logo
// each clan ships for red and blue.
func ClanIconPath(zipPath string) (key, team string, ok bool) {
	lower := strings.ToLower(zipPath)
	dir, file := path.Split(lower)
	if dir != "team_icon/" || path.Ext(file) != ".tga" {
		return "", "", false
	}
	base := strings.TrimSuffix(file, ".tga")
	for _, t := range []string{"red", "blue"} {
		if name, found := strings.CutSuffix(base, "_"+t); found && name != "" {
			return ClanKey(name), t, true
		}
	}
	return "", "", false
}
