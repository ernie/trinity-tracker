package setup

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

// stemDisplayNames maps each shipped key stem to how it should appear
// in sv_hostname. Operator-edited keys fall through to verbatim use.
var stemDisplayNames = map[string]string{
	"ffa":       "FFA",
	"1v1":       "1v1",
	"tdm":       "TDM",
	"tdm-ta":    "TDM-TA",
	"ctf":       "CTF",
	"ctf-ta":    "CTF-TA",
	"1fctf":     "1FCTF",
	"overload":  "Overload",
	"harvester": "Harvester",
}

// displayHostname turns a key into its "Trinity <NAME>" sv_hostname
// form. Known stems get mapped names; an operator-edited key passes
// through verbatim. Collision suffixes ("-2", "-3", …) are stripped,
// looked up against the stem, and re-appended.
func displayHostname(key string) string {
	if name, ok := stemDisplayNames[key]; ok {
		return "Trinity " + name
	}
	if i := strings.LastIndex(key, "-"); i > 0 {
		stem, suffix := key[:i], key[i:]
		if name, ok := stemDisplayNames[stem]; ok {
			return "Trinity " + name + suffix
		}
	}
	return "Trinity " + key
}

//go:embed cfgtemplates/*.cfg cfgtemplates/*.txt cfgtemplates/rotations/*
var cfgTemplates embed.FS

//go:embed systemd/*
var systemdUnits embed.FS

// Gametype is the engine's g_gametype value. Mod folder (baseq3 vs
// missionpack) is a separate axis on ServerAnswers.
type Gametype int

const (
	GametypeFFA        Gametype = 0
	GametypeTournament Gametype = 1
	// 2 is "Single Player" — not exposed to operators.
	GametypeTDM       Gametype = 3
	GametypeCTF       Gametype = 4
	GametypeOneFlag   Gametype = 5
	GametypeOverload  Gametype = 6
	GametypeHarvester Gametype = 7
)

// IsTeamArenaOnly reports whether the gametype only exists in TA's
// missionpack mod (operator can't pick baseq3 for it).
func (g Gametype) IsTeamArenaOnly() bool {
	switch g {
	case GametypeOneFlag, GametypeOverload, GametypeHarvester:
		return true
	}
	return false
}

// Label returns the canonical short name. " (TA)" is appended by
// GametypeChoice when the menu entry runs under missionpack.
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
		return "One Flag CTF"
	case GametypeOverload:
		return "Overload"
	case GametypeHarvester:
		return "Harvester"
	default:
		return fmt.Sprintf("gametype(%d)", g)
	}
}

// GametypeChoice is one (gametype, mod) menu entry.
type GametypeChoice struct {
	Gametype       Gametype
	UseMissionpack bool
}

func (c GametypeChoice) Label() string {
	if c.UseMissionpack {
		return c.Gametype.Label() + " (TA)"
	}
	return c.Gametype.Label()
}

// GametypeChoices is the wizard's menu list.
var GametypeChoices = []GametypeChoice{
	{GametypeFFA, false},
	{GametypeTournament, false},
	{GametypeTDM, false},
	{GametypeTDM, true},
	{GametypeCTF, false},
	{GametypeCTF, true},
	{GametypeOneFlag, true},
	{GametypeOverload, true},
	{GametypeHarvester, true},
}

// hasTAVariant reports whether the gametype has a -ta.cfg / -ta
// rotation alongside the baseq3 default. Only TDM and CTF do.
func (g Gametype) hasTAVariant() bool {
	return g == GametypeTDM || g == GametypeCTF
}

func cfgTemplateFor(g Gametype, useMissionpack bool) string {
	if s := Stem(g, useMissionpack); s != "" {
		return s + ".cfg"
	}
	return ""
}

func rotationTemplateFor(g Gametype, useMissionpack bool) string {
	return Stem(g, useMissionpack)
}

// Stem is the lowercase identifier used both as the per-gametype cfg
// filename (and rotation filename) and as the wizard's default key
// when the operator hasn't typed anything.
func Stem(g Gametype, useMissionpack bool) string {
	base := bareStem(g)
	if base == "" {
		return ""
	}
	if useMissionpack && g.hasTAVariant() {
		return base + "-ta"
	}
	return base
}

func bareStem(g Gametype) string {
	switch g {
	case GametypeFFA:
		return "ffa"
	case GametypeTournament:
		return "1v1"
	case GametypeTDM:
		return "tdm"
	case GametypeCTF:
		return "ctf"
	case GametypeOneFlag:
		return "1fctf"
	case GametypeOverload:
		return "overload"
	case GametypeHarvester:
		return "harvester"
	default:
		return ""
	}
}

// RenderServerCfg fills the shared-per-gametype cfg template.
// {{stem}} → the cfg's filename stem (e.g. "tdm-ta"); {{hostname}} →
// "Trinity <DISPLAY>" derived from the same stem; {{rcon_password}} →
// the rcon credential, which lives here (mode 0640) rather than in
// trinity.cfg or the .env file so it's not exposed via `ps` and not
// world-readable. Operators tweak the rendered file post-install for
// per-instance customization.
func RenderServerCfg(g Gametype, useMissionpack bool, rconPassword string) (string, error) {
	name := cfgTemplateFor(g, useMissionpack)
	if name == "" {
		return "", fmt.Errorf("no template for gametype %d", int(g))
	}
	raw, err := cfgTemplates.ReadFile("cfgtemplates/" + name)
	if err != nil {
		return "", fmt.Errorf("reading template %s: %w", name, err)
	}
	stem := Stem(g, useMissionpack)
	out := strings.ReplaceAll(string(raw), "{{stem}}", stem)
	out = strings.ReplaceAll(out, "{{hostname}}", displayHostname(stem))
	out = strings.ReplaceAll(out, "{{rcon_password}}", rconPassword)
	return out, nil
}

// RenderRotation returns the default map list for the (gametype, mod)
// pair. Stock pak0 maps only; operators edit rotation.<stem> in the q3
// fs to add custom maps.
func RenderRotation(g Gametype, useMissionpack bool) ([]byte, error) {
	name := rotationTemplateFor(g, useMissionpack)
	if name == "" {
		return nil, fmt.Errorf("no rotation for gametype %d", int(g))
	}
	raw, err := cfgTemplates.ReadFile("cfgtemplates/rotations/" + name)
	if err != nil {
		return nil, fmt.Errorf("reading rotation %s: %w", name, err)
	}
	return raw, nil
}

// TrinityBotsFile returns the curated bot definitions list shipped
// alongside the wizard. Installed at <quake3>/baseq3/scripts/trinity-bots.txt
// and referenced from quake3-server@.service via `+set g_botsfile`.
// `bot_minplayers` reads this file to fill empty slots.
func TrinityBotsFile() ([]byte, error) {
	raw, err := cfgTemplates.ReadFile("cfgtemplates/trinity-bots.txt")
	if err != nil {
		return nil, fmt.Errorf("reading trinity-bots.txt template: %w", err)
	}
	return raw, nil
}

// RenderTrinityCfg fills the trinity.cfg template (required cvars
// shared across servers). publicURL is used to derive the optional
// fast-download block and the default g_motd; pass an empty string
// to omit fastdl and leave the MOTD blank (collector-only installs
// without nginx, or hub-only installs). rconpassword is intentionally
// not set here — see the per-server +set in installPerServerFiles.
func RenderTrinityCfg(publicURL string) (string, error) {
	raw, err := cfgTemplates.ReadFile("cfgtemplates/trinity.cfg")
	if err != nil {
		return "", fmt.Errorf("reading trinity.cfg template: %w", err)
	}
	out := strings.ReplaceAll(string(raw), "{{fastdl_block}}", fastdlBlock(publicURL))
	out = strings.ReplaceAll(out, "{{motd_host}}", HostFromURL(publicURL))
	return out, nil
}

// fastdlBlock returns the sv_dlURL/sv_tvDownload cvar lines if a
// public URL is available, or a commented-out reminder otherwise.
// The nginx fastdl vhost lives on dl.<host>.
func fastdlBlock(publicURL string) string {
	host := HostFromURL(publicURL)
	if host == "" {
		return `// (Set sv_dlURL to your collector's dl.<hostname> to enable
// fast-download. Without it, players can't fetch maps they don't
// already have.)
// set sv_tvDownload         1
// set sv_dlURL              "https://dl.your-hostname"`
	}
	return fmt.Sprintf(`set sv_tvDownload         1
set sv_dlURL              "https://dl.%s"`, host)
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
// stable order. Used by Apply to copy them all into
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
