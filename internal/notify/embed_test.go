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
			{Name: "^1ernie", Score: 12, IsBot: false},
			{Name: "rocketeer", Score: 9, IsBot: false},
			{Name: "Sarge", Score: 5, IsBot: true},
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

// Map gets its long name when mapMeta has it; short id when not.
func TestBuildActiveEmbed_MapLongName(t *testing.T) {
	mapMeta := map[string]string{"q3dm17": "The Longest Yard"}
	embed := buildActiveEmbed(mkStatus(), "https://trinity.example.com", mapMeta)
	if !strings.Contains(embed.Description, "The Longest Yard (q3dm17)") {
		t.Errorf("description should include long+short: %q", embed.Description)
	}
}

// Roster groups humans before bots, score-sorted descending; bot
// rows get a "(bot)" suffix so the channel doesn't think a bot is
// the headline player.
func TestBuildActiveEmbed_RosterShape(t *testing.T) {
	embed := buildActiveEmbed(mkStatus(), "", nil)
	if len(embed.Fields) != 1 {
		t.Fatalf("expected 1 roster field, got %d", len(embed.Fields))
	}
	val := embed.Fields[0].Value
	idxErnie := strings.Index(val, "ernie")
	idxRocketeer := strings.Index(val, "rocketeer")
	idxSarge := strings.Index(val, "Sarge")
	if idxErnie < 0 || idxRocketeer < 0 || idxSarge < 0 {
		t.Fatalf("roster missing player(s): %q", val)
	}
	if !(idxErnie < idxRocketeer && idxRocketeer < idxSarge) {
		t.Errorf("expected order ernie < rocketeer < sarge in roster, got %q", val)
	}
	if !strings.Contains(val, "(bot)") {
		t.Errorf("bot row should be marked (bot): %q", val)
	}
	// Q3-colored ^1 should translate to a 31m ANSI escape in the block.
	if !strings.Contains(val, "\x1b[31m") {
		t.Errorf("colored ernie name missing ANSI escape: %q", val)
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
}
