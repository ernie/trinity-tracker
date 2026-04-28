package setup

import (
	"strings"
	"testing"
)

func TestRenderServerCfg_FFA(t *testing.T) {
	out, err := RenderServerCfg(GametypeFFA, "ffa")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, "g_gametype      0") {
		t.Errorf("missing g_gametype 0 in:\n%s", out)
	}
	if !strings.Contains(out, "FFA") {
		t.Errorf("expected upper-cased {{hostname}} substitution containing FFA, got:\n%s", out)
	}
	if strings.Contains(out, "{{") {
		t.Errorf("unsubstituted placeholder remains:\n%s", out)
	}
}

func TestRenderServerCfg_Tournament(t *testing.T) {
	out, err := RenderServerCfg(GametypeTournament, "1v1")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, "g_gametype      1") {
		t.Errorf("missing g_gametype 1, got:\n%s", out)
	}
}

func TestRenderServerCfg_TeamArenaGametypes(t *testing.T) {
	cases := []struct {
		g    Gametype
		want string
	}{
		{GametypeOneFlag, "g_gametype      5"},
		{GametypeOverload, "g_gametype      6"},
		{GametypeHarvester, "g_gametype      7"},
	}
	for _, tc := range cases {
		t.Run(tc.g.Label(), func(t *testing.T) {
			out, err := RenderServerCfg(tc.g, "ta")
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("missing %q in:\n%s", tc.want, out)
			}
			if !strings.Contains(out, "missionpack") {
				t.Errorf("expected missionpack note for %s in:\n%s", tc.g.Label(), out)
			}
		})
	}
}

func TestGametype_IsMissionpack(t *testing.T) {
	for _, g := range []Gametype{GametypeOneFlag, GametypeOverload, GametypeHarvester} {
		if !g.IsMissionpack() {
			t.Errorf("%s should be missionpack", g.Label())
		}
		if g.ModFolder() != "missionpack" {
			t.Errorf("%s mod folder: got %q", g.Label(), g.ModFolder())
		}
	}
	for _, g := range []Gametype{GametypeFFA, GametypeTournament, GametypeTDM, GametypeCTF} {
		if g.IsMissionpack() {
			t.Errorf("%s should not be missionpack", g.Label())
		}
		if g.ModFolder() != "baseq3" {
			t.Errorf("%s mod folder: got %q", g.Label(), g.ModFolder())
		}
	}
}

func TestRenderTrinityCfg_WithPublicURL(t *testing.T) {
	out, err := RenderTrinityCfg("hunter2", "https://q3.example.com")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, `"hunter2"`) {
		t.Errorf("rcon password not substituted, got:\n%s", out)
	}
	if !strings.Contains(out, "q3.example.com:27970") {
		t.Errorf("expected fastdl URL with q3.example.com, got:\n%s", out)
	}
	if strings.Contains(out, "{{") {
		t.Errorf("unsubstituted placeholder, got:\n%s", out)
	}
}

func TestRenderTrinityCfg_WithoutPublicURL_LeavesFastdlCommented(t *testing.T) {
	out, err := RenderTrinityCfg("hunter2", "")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, "// set sv_dlURL") {
		t.Errorf("expected commented sv_dlURL when public URL is empty, got:\n%s", out)
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
