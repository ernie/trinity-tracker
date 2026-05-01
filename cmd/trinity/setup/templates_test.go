package setup

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func hasGametype(out string, n int) bool {
	re := regexp.MustCompile(`(?m)^\s*set\s+g_gametype\s+` + strconv.Itoa(n) + `\b`)
	return re.MatchString(out)
}

func TestRenderServerCfg_FFA(t *testing.T) {
	out, err := RenderServerCfg(GametypeFFA, false, "test-rcon")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !hasGametype(out, 0) {
		t.Errorf("missing g_gametype 0 in:\n%s", out)
	}
	if !strings.Contains(out, `"Trinity FFA"`) {
		t.Errorf("expected hostname \"Trinity FFA\", got:\n%s", out)
	}
	if strings.Contains(out, "{{") {
		t.Errorf("unsubstituted placeholder remains:\n%s", out)
	}
}

func TestRenderServerCfg_Tournament(t *testing.T) {
	out, err := RenderServerCfg(GametypeTournament, false, "test-rcon")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !hasGametype(out, 1) {
		t.Errorf("missing g_gametype 1, got:\n%s", out)
	}
	if !strings.Contains(out, `"Trinity 1v1"`) {
		t.Errorf("expected hostname \"Trinity 1v1\" (lowercase v), got:\n%s", out)
	}
	if strings.Contains(out, "1V1") {
		t.Errorf("hostname should not upcase v in NvN; got:\n%s", out)
	}
}

func TestDisplayHostname(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ffa", "Trinity FFA"},
		{"tdm", "Trinity TDM"},
		{"tdm-ta", "Trinity TDM-TA"},
		{"tdm-ta-2", "Trinity TDM-TA-2"},
		{"1v1", "Trinity 1v1"},
		{"1v1-2", "Trinity 1v1-2"},
		{"1fctf", "Trinity 1FCTF"},
		{"overload", "Trinity Overload"},
		{"overload-2", "Trinity Overload-2"},
		{"harvester", "Trinity Harvester"},
		{"playoffs", "Trinity playoffs"}, // unknown stem → verbatim
	}
	for _, tc := range cases {
		if got := displayHostname(tc.in); got != tc.want {
			t.Errorf("displayHostname(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRenderServerCfg_TeamArenaGametypes(t *testing.T) {
	cases := []struct {
		g    Gametype
		want int
	}{
		{GametypeOneFlag, 5},
		{GametypeOverload, 6},
		{GametypeHarvester, 7},
	}
	for _, tc := range cases {
		t.Run(tc.g.Label(), func(t *testing.T) {
			out, err := RenderServerCfg(tc.g, true, "test-rcon")
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if !hasGametype(out, tc.want) {
				t.Errorf("missing g_gametype %d in:\n%s", tc.want, out)
			}
			if !strings.Contains(out, "missionpack") {
				t.Errorf("expected missionpack note for %s in:\n%s", tc.g.Label(), out)
			}
		})
	}
}

func TestGametype_IsTeamArenaOnly(t *testing.T) {
	for _, g := range []Gametype{GametypeOneFlag, GametypeOverload, GametypeHarvester} {
		if !g.IsTeamArenaOnly() {
			t.Errorf("%s should be TA-only", g.Label())
		}
	}
	for _, g := range []Gametype{GametypeFFA, GametypeTournament, GametypeTDM, GametypeCTF} {
		if g.IsTeamArenaOnly() {
			t.Errorf("%s should not be TA-only", g.Label())
		}
	}
}

// TestStartingMapIsInRotation guards against a footgun where the cfg
// boots a map that's not in g_rotation — the operator would never see
// it again after the first match ends, surprising them.
func TestStartingMapIsInRotation(t *testing.T) {
	for _, c := range GametypeChoices {
		t.Run(c.Label(), func(t *testing.T) {
			cfg, err := RenderServerCfg(c.Gametype, c.UseMissionpack, "test-rcon")
			if err != nil {
				t.Fatalf("cfg: %v", err)
			}
			rot, err := RenderRotation(c.Gametype, c.UseMissionpack)
			if err != nil {
				t.Fatalf("rotation: %v", err)
			}
			startMap := ""
			for _, line := range strings.Split(cfg, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "map ") {
					startMap = strings.TrimSpace(strings.TrimPrefix(line, "map"))
					break
				}
			}
			if startMap == "" {
				t.Fatalf("no `map X` line in rendered cfg")
			}
			rotationMaps := map[string]bool{}
			for _, line := range strings.Split(string(rot), "\n") {
				if m := strings.TrimSpace(line); m != "" {
					rotationMaps[m] = true
				}
			}
			if !rotationMaps[startMap] {
				t.Errorf("starting map %q not in rotation %v", startMap, rotationMaps)
			}
		})
	}
}

func TestServerAnswers_ModFolder(t *testing.T) {
	cases := []struct {
		s    ServerAnswers
		want string
	}{
		{ServerAnswers{Gametype: GametypeFFA}, "baseq3"},
		{ServerAnswers{Gametype: GametypeTDM}, "baseq3"},
		{ServerAnswers{Gametype: GametypeTDM, UseMissionpack: true}, "missionpack"},
		{ServerAnswers{Gametype: GametypeCTF, UseMissionpack: true}, "missionpack"},
		{ServerAnswers{Gametype: GametypeOneFlag}, "missionpack"},
		{ServerAnswers{Gametype: GametypeOverload}, "missionpack"},
	}
	for _, tc := range cases {
		if got := tc.s.ModFolder(); got != tc.want {
			t.Errorf("%v ModFolder = %q, want %q", tc.s, got, tc.want)
		}
	}
}

func TestRenderTrinityCfg_WithPublicURL(t *testing.T) {
	out, err := RenderTrinityCfg("https://q3.example.com")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, `"https://dl.q3.example.com"`) {
		t.Errorf("expected fastdl URL https://dl.q3.example.com, got:\n%s", out)
	}
	if strings.Contains(out, "{{") {
		t.Errorf("unsubstituted placeholder, got:\n%s", out)
	}
	if strings.Contains(out, "set rconpassword") {
		t.Errorf("trinity.cfg must not set rconpassword (per-stem cfg owns that), got:\n%s", out)
	}
}

func TestRenderTrinityCfg_WithoutPublicURL_LeavesFastdlCommented(t *testing.T) {
	out, err := RenderTrinityCfg("")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, "// set sv_dlURL") {
		t.Errorf("expected commented sv_dlURL when public URL is empty, got:\n%s", out)
	}
}

func TestRenderServerCfg_EmbedsRconPassword(t *testing.T) {
	out, err := RenderServerCfg(GametypeFFA, false, "hunter2")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, `set rconpassword       "hunter2"`) {
		t.Errorf("rcon password not substituted in stem cfg, got:\n%s", out)
	}
	if strings.Contains(out, "{{rcon_password}}") {
		t.Errorf("unsubstituted rcon placeholder, got:\n%s", out)
	}
}

func TestHostFromURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://q3.example.com", "q3.example.com"},
		{"http://q3.example.com:8080/path", "q3.example.com"},
		{"q3.example.com", "q3.example.com"},
		{"", ""},
		{"https://q3.example.com/", "q3.example.com"},
	}
	for _, tc := range cases {
		got := HostFromURL(tc.in)
		if got != tc.want {
			t.Errorf("HostFromURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSystemdUnit_DefaultUser_Unmodified(t *testing.T) {
	data, err := SystemdUnit("trinity.service", "quake")
	if err != nil {
		t.Fatalf("SystemdUnit: %v", err)
	}
	if !strings.Contains(string(data), "User=quake") {
		t.Errorf("expected User=quake")
	}
}

func TestSystemdUnit_CustomUser_Substitutes(t *testing.T) {
	data, err := SystemdUnit("trinity.service", "tracker")
	if err != nil {
		t.Fatalf("SystemdUnit: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "User=tracker") {
		t.Errorf("expected User=tracker")
	}
	if strings.Contains(body, "User=quake") {
		t.Errorf("unexpected User=quake in custom-user output")
	}
}

func TestListSystemdUnits(t *testing.T) {
	names, err := ListSystemdUnits()
	if err != nil {
		t.Fatalf("ListSystemdUnits: %v", err)
	}
	want := map[string]bool{
		"trinity.service":         true,
		"quake3-server@.service":  true,
		"quake3-servers.target":   true,
	}
	got := map[string]bool{}
	for _, n := range names {
		got[n] = true
	}
	for k := range want {
		if !got[k] {
			t.Errorf("missing unit %q in %v", k, names)
		}
	}
}
