package setup

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed cfgtemplates/*.cfg
var cfgTemplates embed.FS

//go:embed systemd/*
var systemdUnits embed.FS

// Gametype is the q3 g_gametype value plus enough metadata to pick
// the right cfg template and decide whether the server runs against
// missionpack instead of baseq3.
type Gametype int

const (
	GametypeFFA        Gametype = 0
	GametypeTournament Gametype = 1
	// 2 is "Single Player" in the engine — not exposed to operators.
	GametypeTDM       Gametype = 3
	GametypeCTF       Gametype = 4
	GametypeOneFlag   Gametype = 5 // from Team Arena
	GametypeOverload  Gametype = 6 // from Team Arena
	GametypeHarvester Gametype = 7 // from Team Arena
)

// Gametypes lists the operator-selectable gametypes in menu order.
var Gametypes = []Gametype{
	GametypeFFA,
	GametypeTournament,
	GametypeTDM,
	GametypeCTF,
	GametypeOneFlag,
	GametypeOverload,
	GametypeHarvester,
}

// Label returns the menu label for a gametype.
func (g Gametype) Label() string {
	switch g {
	case GametypeFFA:
		return "Free-For-All"
	case GametypeTournament:
		return "Tournament (1v1)"
	case GametypeTDM:
		return "Team Deathmatch"
	case GametypeCTF:
		return "Capture The Flag"
	case GametypeOneFlag:
		return "One Flag CTF (from Team Arena)"
	case GametypeOverload:
		return "Overload (from Team Arena)"
	case GametypeHarvester:
		return "Harvester (from Team Arena)"
	default:
		return fmt.Sprintf("gametype(%d)", g)
	}
}

// templateFile is the relative filename inside cfgtemplates/ that
// renders into the per-server <key>.cfg.
func (g Gametype) templateFile() string {
	switch g {
	case GametypeFFA:
		return "ffa.cfg"
	case GametypeTournament:
		return "tournament.cfg"
	case GametypeTDM:
		return "tdm.cfg"
	case GametypeCTF:
		return "ctf.cfg"
	case GametypeOneFlag:
		return "oneflag.cfg"
	case GametypeOverload:
		return "overload.cfg"
	case GametypeHarvester:
		return "harvester.cfg"
	default:
		return ""
	}
}

// IsMissionpack reports whether the gametype is from Team Arena
// (and so requires the missionpack base assets). Affects fs_game in
// the .env file and the destination directory of the rendered
// <key>.cfg.
func (g Gametype) IsMissionpack() bool {
	return g == GametypeOneFlag || g == GametypeOverload || g == GametypeHarvester
}

// ModFolder returns "missionpack" or "baseq3".
func (g Gametype) ModFolder() string {
	if g.IsMissionpack() {
		return "missionpack"
	}
	return "baseq3"
}

// RenderServerCfg fills out the gametype's cfg template for one
// server. The {{key}} placeholder becomes the lower-case key; the
// {{hostname}} placeholder gets the human-friendly upper-case key
// (operators tweak it after install if they want a fancier name).
func RenderServerCfg(g Gametype, key string) (string, error) {
	name := g.templateFile()
	if name == "" {
		return "", fmt.Errorf("no template for gametype %d", int(g))
	}
	raw, err := cfgTemplates.ReadFile("cfgtemplates/" + name)
	if err != nil {
		return "", fmt.Errorf("reading template %s: %w", name, err)
	}
	out := strings.ReplaceAll(string(raw), "{{key}}", strings.ToLower(key))
	out = strings.ReplaceAll(out, "{{hostname}}", strings.ToUpper(key))
	return out, nil
}

// RenderTrinityCfg fills the trinity.cfg template (required cvars
// shared across servers). publicURL is used to derive the optional
// fast-download block; pass an empty string to omit it (collector-only
// installs without nginx, or hub-only installs).
func RenderTrinityCfg(rconPassword, publicURL string) (string, error) {
	raw, err := cfgTemplates.ReadFile("cfgtemplates/trinity.cfg")
	if err != nil {
		return "", fmt.Errorf("reading trinity.cfg template: %w", err)
	}
	out := strings.ReplaceAll(string(raw), "{{rcon_password}}", rconPassword)
	out = strings.ReplaceAll(out, "{{fastdl_block}}", fastdlBlock(publicURL))
	return out, nil
}

// fastdlBlock returns the sv_dlURL/sv_tvDownload cvar lines if a
// public URL is available, or a commented-out reminder otherwise. The
// nginx fast-dl vhost (set up by bootstrap-nginx.sh) listens on plain
// :27970 so we hardcode the port and swap http for https.
func fastdlBlock(publicURL string) string {
	host := HostFromURL(publicURL)
	if host == "" {
		return `// (Run scripts/bootstrap-nginx.sh and set sv_dlURL to its hostname:27970
// here to enable fast-download. Without it, players can't fetch maps
// they don't already have.)
// set sv_tvDownload         1
// set sv_dlURL              "http://your-hostname:27970"`
	}
	return fmt.Sprintf(`set sv_tvDownload         1
set sv_dlURL              "http://%s:27970"`, host)
}

// HostFromURL extracts the bare hostname from an http(s) URL. Returns
// empty if the URL is malformed or has no host.
func HostFromURL(s string) string {
	if s == "" {
		return ""
	}
	// url.Parse is lenient — strip the scheme manually for robustness
	// against operators who paste "example.com" without "https://".
	rest := s
	if i := strings.Index(rest, "://"); i >= 0 {
		rest = rest[i+3:]
	}
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}
	if i := strings.Index(rest, ":"); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

// SystemdUnit returns the embedded unit file content with the User=
// and Group= lines patched to match the requested service user.
// units names are without the "systemd/" prefix, e.g.
// "trinity.service".
func SystemdUnit(name, serviceUser string) ([]byte, error) {
	raw, err := systemdUnits.ReadFile("systemd/" + name)
	if err != nil {
		return nil, fmt.Errorf("reading embedded systemd/%s: %w", name, err)
	}
	if serviceUser == "" || serviceUser == "quake" {
		return raw, nil
	}
	out := strings.ReplaceAll(string(raw), "User=quake", "User="+serviceUser)
	out = strings.ReplaceAll(out, "Group=quake", "Group="+serviceUser)
	return []byte(out), nil
}

// ListSystemdUnits returns the names of every embedded unit, in a
// stable order. Used by the actuator to copy them all into
// /etc/systemd/system.
func ListSystemdUnits() ([]string, error) {
	entries, err := fs.ReadDir(systemdUnits, "systemd")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
