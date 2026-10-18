package notify

import (
	"strings"
	"testing"

	"github.com/ernie/trinity-tracker/internal/domain"
)

func mkStatus() *domain.ServerStatus {
	return &domain.ServerStatus{
		ServerID: 42,
		Source:   "local",
		Key:      "dm-hub",
		Map:      "q3dm17",
		GameType: "FFA",
		Players: []domain.PlayerStatus{
			{Name: "rocketeer", IsBot: false},
			{Name: "^1alice", IsBot: false},
			{Name: "Sarge", IsBot: true},
		},
	}
}

func TestBuildActiveEmbed_TitleAndIdentity(t *testing.T) {
	embed := buildActiveEmbed(mkStatus(), "https://trinity.example.com", nil)
	if embed.Title != "🟢  local / dm-hub is active" {
		t.Errorf("title: got %q", embed.Title)
	}
	if embed.URL != "https://trinity.example.com/servers" {
		t.Errorf("url: got %q", embed.URL)
	}
	if embed.Color != activeColor {
		t.Errorf("color: got %#x, want %#x", embed.Color, activeColor)
	}
}

func TestBuildActiveEmbed_LiveLink(t *testing.T) {
	s := mkStatus()
	s.IsLive = true
	embed := buildActiveEmbed(s, "https://trinity.example.com", nil)
	if embed.URL != "https://trinity.example.com/tv/local/dm-hub" {
		t.Errorf("live title should link to the player: %q", embed.URL)
	}
	if !strings.Contains(embed.Title, "(click to watch)") {
		t.Errorf("live title should invite the click: %q", embed.Title)
	}
}

func TestBuildActiveEmbed_NoLiveLinkWhenNotLive(t *testing.T) {
	embed := buildActiveEmbed(mkStatus(), "https://trinity.example.com", nil)
	if embed.URL != "https://trinity.example.com/servers" {
		t.Errorf("non-live title should link to the server list: %q", embed.URL)
	}
	if strings.Contains(embed.Title, "click to watch") {
		t.Errorf("non-live server should not invite a watch: %q", embed.Title)
	}
}

func TestBuildActiveEmbed_NoLiveLinkWithoutPublicURL(t *testing.T) {
	s := mkStatus()
	s.IsLive = true
	embed := buildActiveEmbed(s, "", nil)
	if embed.URL != "" {
		t.Errorf("without a public URL there is no link: %q", embed.URL)
	}
	if strings.Contains(embed.Title, "click to watch") {
		t.Errorf("without a public URL there is nothing to watch: %q", embed.Title)
	}
}

func TestBuildActiveEmbed_MapLongName(t *testing.T) {
	mapMeta := map[string]string{"q3dm17": "The Longest Yard"}
	embed := buildActiveEmbed(mkStatus(), "https://trinity.example.com", mapMeta)
	if !strings.Contains(embed.Description, "The Longest Yard (q3dm17)") {
		t.Errorf("description should include long+short: %q", embed.Description)
	}
}

// Humans come before bots, each sorted by name, and bots are marked.
func TestBuildActiveEmbed_RosterShape(t *testing.T) {
	embed := buildActiveEmbed(mkStatus(), "", nil)
	if len(embed.Fields) != 1 {
		t.Fatalf("expected 1 roster field, got %d", len(embed.Fields))
	}
	val := embed.Fields[0].Value
	idxAlice := strings.Index(val, "alice")
	idxRocketeer := strings.Index(val, "rocketeer")
	idxSarge := strings.Index(val, "Sarge")
	if idxAlice < 0 || idxRocketeer < 0 || idxSarge < 0 {
		t.Fatalf("roster missing player(s): %q", val)
	}
	if !(idxAlice < idxRocketeer && idxRocketeer < idxSarge) {
		t.Errorf("expected order alice < rocketeer < sarge in roster, got %q", val)
	}
	if !strings.Contains(val, "(bot)") {
		t.Errorf("bot row should be marked (bot): %q", val)
	}
	// ^1 becomes the 31m ANSI escape.
	if !strings.Contains(val, "\x1b[31m") {
		t.Errorf("colored alice name missing ANSI escape: %q", val)
	}
}

func TestBuildActiveEmbed_PlayerCounts(t *testing.T) {
	embed := buildActiveEmbed(mkStatus(), "", nil)
	if !strings.Contains(embed.Description, "2 humans") {
		t.Errorf("expected '2 humans' in description: %q", embed.Description)
	}
	if !strings.Contains(embed.Description, "1 bot") {
		t.Errorf("expected '1 bot' (singular) in description: %q", embed.Description)
	}
}

func TestBuildActiveEmbed_TeamModeColumns(t *testing.T) {
	s := &domain.ServerStatus{
		ServerID: 42, Source: "local", Key: "dm-hub", Map: "q3ctf3", GameType: "CTF",
		TeamScores: &domain.TeamScores{RedScore: 3, BlueScore: 1},
		Players: []domain.PlayerStatus{
			{Name: "red-ace", IsBot: false, Team: 1},
			{Name: "redbot", IsBot: true, Team: 1},
			{Name: "blue-pro", IsBot: false, Team: 2},
		},
	}
	embed := buildActiveEmbed(s, "", nil)
	if len(embed.Fields) != 2 {
		t.Fatalf("expected 2 fields (Red, Blue), got %d: %+v", len(embed.Fields), embed.Fields)
	}
	if embed.Fields[0].Name != "Red" || !embed.Fields[0].Inline {
		t.Errorf("field 0: want inline Red, got %+v", embed.Fields[0])
	}
	if embed.Fields[1].Name != "Blue" || !embed.Fields[1].Inline {
		t.Errorf("field 1: want inline Blue, got %+v", embed.Fields[1])
	}
	if !strings.Contains(embed.Fields[0].Value, "red-ace") || !strings.Contains(embed.Fields[0].Value, "redbot") {
		t.Errorf("Red field missing players: %q", embed.Fields[0].Value)
	}
	if !strings.Contains(embed.Fields[1].Value, "blue-pro") {
		t.Errorf("Blue field missing player: %q", embed.Fields[1].Value)
	}
}

func TestBuildActiveEmbed_SpectatorsField(t *testing.T) {
	s := &domain.ServerStatus{
		ServerID: 42, Source: "local", Key: "dm-hub", Map: "q3ctf3", GameType: "CTF",
		Players: []domain.PlayerStatus{
			{Name: "red-ace", IsBot: false, Team: 1},
			{Name: "blue-pro", IsBot: false, Team: 2},
			{Name: "watcher", IsBot: false, Team: 3},
		},
	}
	embed := buildActiveEmbed(s, "", nil)
	if len(embed.Fields) != 3 {
		t.Fatalf("expected 3 fields (Red, Blue, Spectators), got %d", len(embed.Fields))
	}
	spec := embed.Fields[2]
	if spec.Name != "Spectators" {
		t.Errorf("field 2 name: got %q", spec.Name)
	}
	if spec.Inline {
		t.Errorf("Spectators field should not be inline (own row)")
	}
	if !strings.Contains(spec.Value, "watcher") {
		t.Errorf("Spectators field missing watcher: %q", spec.Value)
	}
}

func TestBuildActiveEmbed_FFATwoColumns(t *testing.T) {
	s := &domain.ServerStatus{
		ServerID: 42, Source: "local", Key: "dm-hub", Map: "q3dm17", GameType: "FFA",
		Players: []domain.PlayerStatus{
			{Name: "alpha"}, {Name: "bravo"}, {Name: "charlie"}, {Name: "delta"},
		},
	}
	embed := buildActiveEmbed(s, "", nil)
	if len(embed.Fields) != 1 || embed.Fields[0].Name != "Players" {
		t.Fatalf("want single Players field, got %+v", embed.Fields)
	}
	if embed.Fields[0].Inline {
		t.Errorf("FFA Players field should not be inline")
	}
	val := embed.Fields[0].Value
	var alphaLine string
	for _, line := range strings.Split(val, "\n") {
		if strings.Contains(line, "alpha") {
			alphaLine = line
			break
		}
	}
	if !strings.Contains(alphaLine, "bravo") {
		t.Errorf("two-column layout: alpha+bravo should share a line, got %q", alphaLine)
	}
}

func TestBuildActiveEmbed_StripsVRTag(t *testing.T) {
	s := &domain.ServerStatus{
		ServerID: 42, Source: "local", Key: "dm-hub", Map: "q3dm17", GameType: "FFA",
		Players: []domain.PlayerStatus{
			{Name: "[VR] phoenix", IsVR: true},
			{Name: "raven [VR]", IsVR: true},
		},
	}
	embed := buildActiveEmbed(s, "", nil)
	if len(embed.Fields) != 1 {
		t.Fatalf("want 1 field, got %d", len(embed.Fields))
	}
	val := embed.Fields[0].Value
	if strings.Contains(val, "[VR]") {
		t.Errorf("VR tag should be stripped from display: %q", val)
	}
	if !strings.Contains(val, "phoenix") || !strings.Contains(val, "raven") {
		t.Errorf("stripped names should still appear: %q", val)
	}
}

func TestBuildInactiveEmbed_TitleAndDesc(t *testing.T) {
	embed := buildInactiveEmbed(mkStatus(), "https://trinity.example.com", nil)
	if embed.Title != "⚪  local / dm-hub is idle" {
		t.Errorf("title: got %q", embed.Title)
	}
	if !strings.Contains(embed.Description, "q3dm17") {
		t.Errorf("description should mention last map: %q", embed.Description)
	}
	if embed.Color != inactiveColor {
		t.Errorf("color: got %#x", embed.Color)
	}
	if embed.URL != "" {
		t.Errorf("idle embed should have no link: %q", embed.URL)
	}
}
