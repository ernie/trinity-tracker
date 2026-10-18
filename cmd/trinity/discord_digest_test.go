package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/q3color"
)

func mkLeaderboard(period string, entries ...domain.LeaderboardEntry) *domain.LeaderboardResponse {
	start := time.Date(2026, 4, 29, 20, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 6, 20, 0, 0, 0, time.UTC)
	return &domain.LeaderboardResponse{
		Category:    "frags",
		Period:      period,
		PeriodStart: &start,
		PeriodEnd:   &end,
		Entries:     entries,
	}
}

func mkEntry(rank int, name string, frags int64, kd float64) domain.LeaderboardEntry {
	return domain.LeaderboardEntry{
		Rank:       rank,
		Player:     domain.Player{Name: name, CleanName: stripCarets(name)},
		TotalFrags: frags,
		KDRatio:    kd,
		Captures:   frags / 30, // arbitrary, just so non-zero values appear
	}
}

func stripCarets(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '^' && i+1 < len(s) {
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestRenderDigestEmbed_FullWeek(t *testing.T) {
	now := time.Date(2026, 5, 6, 20, 0, 0, 0, time.UTC)
	results := map[string]*domain.LeaderboardResponse{
		"frags":    mkLeaderboard("week", mkEntry(1, "^1alice", 412, 2.84), mkEntry(2, "rocketeer", 318, 2.10)),
		"kd_ratio": mkLeaderboard("week", mkEntry(1, "^1alice", 412, 2.84)),
	}
	embed := renderDigestEmbed(results, []string{"frags", "kd_ratio"}, 5,
		"https://trinity.example.com/leaderboard?period=week&as_of=2026-05-06T20:00:00Z",
		"https://trinity.example.com", "", now)

	if embed.Title != "This week's leaderboard" {
		t.Errorf("title: got %q", embed.Title)
	}
	if !strings.Contains(embed.URL, "as_of=") {
		t.Errorf("URL missing as_of: %q", embed.URL)
	}
	// 2 fields + 1 spacer.
	if len(embed.Fields) != 3 {
		t.Fatalf("expected 3 fields (2 real + 1 spacer), got %d", len(embed.Fields))
	}
	if embed.Fields[0].Name != "🔥 Frags" {
		t.Errorf("field 0 name: got %q", embed.Fields[0].Name)
	}
	if !embed.Fields[0].Inline {
		t.Error("fields should be Inline=true so Discord packs them 2/row at fixed width")
	}
	if !strings.Contains(embed.Fields[0].Value, "```ansi") {
		t.Errorf("field value missing ansi block: %q", embed.Fields[0].Value)
	}
	if !strings.Contains(embed.Fields[0].Value, "\x1b[31malice\x1b[0m") {
		t.Errorf("colored name missing ansi escapes: %q", embed.Fields[0].Value)
	}
	if !strings.Contains(embed.Fields[0].Value, "412") {
		t.Errorf("field 0 value missing frag count: %q", embed.Fields[0].Value)
	}
	if !strings.Contains(embed.Fields[1].Value, "2.84") {
		t.Errorf("field 1 value missing kd_ratio: %q", embed.Fields[1].Value)
	}
	if embed.Footer == nil || !strings.Contains(embed.Footer.Text, "⠀") {
		t.Errorf("expected footer to contain U+2800 padding, got %+v", embed.Footer)
	}
	if !strings.Contains(embed.Description, "Past 7 days") {
		t.Errorf("description missing period: %q", embed.Description)
	}
	if embed.Footer == nil || !strings.Contains(embed.Footer.Text, "Generated") {
		t.Errorf("footer missing generation timestamp: %+v", embed.Footer)
	}
}

func TestRenderDigestEmbed_TitleFollowsPeriod(t *testing.T) {
	now := time.Date(2026, 5, 6, 20, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"day":   "Today's leaderboard",
		"week":  "This week's leaderboard",
		"month": "This month's leaderboard",
		"year":  "This year's leaderboard",
		"all":   "All-time leaderboard",
	}
	for period, want := range cases {
		results := map[string]*domain.LeaderboardResponse{
			"frags": mkLeaderboard(period, mkEntry(1, "alice", 10, 1.0)),
		}
		embed := renderDigestEmbed(results, []string{"frags"}, 5, "", "", "", now)
		if embed.Title != want {
			t.Errorf("period %q: title %q, want %q", period, embed.Title, want)
		}
	}
}

func TestRenderDigestEmbed_OddCountAddsTwoTrailingSpacers(t *testing.T) {
	now := time.Date(2026, 5, 6, 20, 0, 0, 0, time.UTC)
	results := map[string]*domain.LeaderboardResponse{
		"frags":     mkLeaderboard("week", mkEntry(1, "a", 100, 1)),
		"kd_ratio":  mkLeaderboard("week", mkEntry(1, "a", 100, 1)),
		"victories": mkLeaderboard("week", mkEntry(1, "a", 100, 1)),
	}
	embed := renderDigestEmbed(results,
		[]string{"frags", "kd_ratio", "victories"}, 5,
		"https://trinity.example.com/leaderboard", "", "", now)
	// 3 real + 1 spacer (after pair 2) + 2 trailing spacers = 6
	if len(embed.Fields) != 6 {
		t.Fatalf("expected 6 fields (3 real + 3 spacer), got %d", len(embed.Fields))
	}
	for _, i := range []int{4, 5} {
		if embed.Fields[i].Name != "​" {
			t.Errorf("field %d should be ZWS spacer, got name=%q", i, embed.Fields[i].Name)
		}
	}
}

func TestRenderDigestEmbed_EmptyCategories(t *testing.T) {
	results := map[string]*domain.LeaderboardResponse{
		"frags":    mkLeaderboard("week"), // no entries
		"captures": nil,                   // missing entirely
	}
	embed := renderDigestEmbed(results, []string{"frags", "captures"}, 5,
		"https://trinity.example.com/leaderboard", "", "", time.Now())

	// 2 placeholder fields + 1 spacer.
	if len(embed.Fields) != 3 {
		t.Fatalf("expected 3 fields (2 real + 1 spacer), got %d", len(embed.Fields))
	}
	if embed.Author != nil {
		t.Errorf("empty week should not set an author, got %+v", embed.Author)
	}
	for i := 0; i < 2; i++ {
		if !strings.Contains(embed.Fields[i].Value, "_(no activity)_") {
			t.Errorf("field %d should show no-activity placeholder: %q", i, embed.Fields[i].Value)
		}
	}
}

func TestRenderDigestEmbed_TopNTruncates(t *testing.T) {
	entries := []domain.LeaderboardEntry{
		mkEntry(1, "a", 100, 1), mkEntry(2, "b", 90, 1), mkEntry(3, "c", 80, 1),
		mkEntry(4, "d", 70, 1), mkEntry(5, "e", 60, 1), mkEntry(6, "f", 50, 1),
	}
	results := map[string]*domain.LeaderboardResponse{
		"frags": mkLeaderboard("week", entries...),
	}
	embed := renderDigestEmbed(results, []string{"frags"}, 3,
		"https://trinity.example.com/leaderboard", "", "", time.Now())
	v := embed.Fields[0].Value
	if !strings.Contains(v, "1.") || !strings.Contains(v, "3.") {
		t.Errorf("missing top entries: %q", v)
	}
	if strings.Contains(v, "4.") || strings.Contains(v, "5.") {
		t.Errorf("topN=3 should not include rank 4+: %q", v)
	}
}

func TestRenderDigestEmbed_UnknownCategorySkipped(t *testing.T) {
	embed := renderDigestEmbed(map[string]*domain.LeaderboardResponse{}, []string{"nonsense", "frags"}, 5,
		"https://trinity.example.com/leaderboard", "", "", time.Now())
	// 1 field + 2 spacers.
	if len(embed.Fields) != 3 || embed.Fields[0].Name != "🔥 Frags" {
		t.Errorf("expected the valid 'frags' field plus 2 trailing spacers, got %+v", embed.Fields)
	}
}

func TestPlayerBadges_StableWidths(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"admin", playerVerificationBadge(true, true), 1},
		{"verified", playerVerificationBadge(true, false), 1},
		{"unverified", playerVerificationBadge(false, false), 1},
		{"vr", playerPlatformBadge(true), 2},
		{"flat", playerPlatformBadge(false), 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := q3color.VisibleWidth(tc.in); got != tc.want {
				t.Errorf("%s width: got %d, want %d (badge=%q)", tc.name, got, tc.want, tc.in)
			}
		})
	}
}

func TestPlayerVerificationBadge_DistinctStates(t *testing.T) {
	a := playerVerificationBadge(true, true)
	v := playerVerificationBadge(true, false)
	u := playerVerificationBadge(false, false)
	if a == v || v == u || a == u {
		t.Errorf("badges must differ: admin=%q verified=%q unverified=%q", a, v, u)
	}
}

func TestRenderDigestEmbed_AuthorFromLeader(t *testing.T) {
	now := time.Date(2026, 5, 6, 20, 0, 0, 0, time.UTC)

	// A portrait on disk for "doom/default".
	tmp := t.TempDir()
	portraitDir := tmp + "/assets/portraits/doom"
	if err := os.MkdirAll(portraitDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(portraitDir+"/icon_default.png", []byte("png"), 0o644); err != nil {
		t.Fatalf("write portrait: %v", err)
	}

	leader := domain.LeaderboardEntry{
		Rank:       1,
		Player:     domain.Player{ID: 21, Name: "alice", CleanName: "alice", Model: "doom/default"},
		TotalFrags: 412,
	}
	results := map[string]*domain.LeaderboardResponse{
		"frags": mkLeaderboard("week", leader),
	}
	embed := renderDigestEmbed(results, []string{"frags"}, 5,
		"https://trinity.example.com/leaderboard?period=week",
		"https://trinity.example.com", tmp, now)
	if embed.Author == nil {
		t.Fatal("expected author when leader has a model")
	}
	wantName := "🥇 alice · most frags"
	if embed.Author.Name != wantName {
		t.Errorf("author.name: got %q, want %q", embed.Author.Name, wantName)
	}
	wantIcon := "https://trinity.example.com/assets/portraits/doom/icon_default.png"
	if embed.Author.IconURL != wantIcon {
		t.Errorf("author.icon_url: got %q, want %q", embed.Author.IconURL, wantIcon)
	}
	wantURL := "https://trinity.example.com/players/21"
	if embed.Author.URL != wantURL {
		t.Errorf("author.url (player profile): got %q, want %q", embed.Author.URL, wantURL)
	}
}

func TestRenderDigestEmbed_AuthorFallsBackOnMissingPortrait(t *testing.T) {
	now := time.Date(2026, 5, 6, 20, 0, 0, 0, time.UTC)
	tmp := t.TempDir() // holds no portraits

	leader := domain.LeaderboardEntry{
		Rank:       1,
		Player:     domain.Player{ID: 99, Name: "newplayer", CleanName: "newplayer", Model: "custom-mod/lava"},
		TotalFrags: 50,
	}
	results := map[string]*domain.LeaderboardResponse{
		"frags": mkLeaderboard("week", leader),
	}
	embed := renderDigestEmbed(results, []string{"frags"}, 5,
		"https://trinity.example.com/leaderboard",
		"https://trinity.example.com", tmp, now)
	if embed.Author == nil {
		t.Fatal("expected author even without portrait")
	}
	wantIcon := "https://trinity.example.com/assets/discord/default-portrait.png"
	if embed.Author.IconURL != wantIcon {
		t.Errorf("missing-portrait fallback: got %q, want %q", embed.Author.IconURL, wantIcon)
	}
	if !strings.Contains(embed.Author.Name, "newplayer") {
		t.Errorf("author.name should contain leader name, got %q", embed.Author.Name)
	}
	if !strings.Contains(embed.Author.Name, "most frags") {
		t.Errorf("author.name should include headline phrase, got %q", embed.Author.Name)
	}
}

func TestRenderDigestEmbed_HeadlineFollowsLeaderCategory(t *testing.T) {
	now := time.Date(2026, 5, 6, 20, 0, 0, 0, time.UTC)
	leader := domain.LeaderboardEntry{
		Rank:       1,
		Player:     domain.Player{ID: 21, Name: "alice", CleanName: "alice"},
		TotalFrags: 412,
		KDRatio:    2.84,
	}
	results := map[string]*domain.LeaderboardResponse{
		"frags":    mkLeaderboard("week", leader),
		"kd_ratio": mkLeaderboard("week", leader),
	}
	cases := []struct {
		first    string
		wantSubs string
	}{
		{"frags", "most frags"},
		{"kd_ratio", "best K/D"},
	}
	for _, tc := range cases {
		t.Run(tc.first, func(t *testing.T) {
			embed := renderDigestEmbed(results, []string{tc.first}, 5,
				"https://trinity.example.com/leaderboard",
				"https://trinity.example.com", "", now)
			if embed.Author == nil {
				t.Fatal("author missing")
			}
			if !strings.Contains(embed.Author.Name, tc.wantSubs) {
				t.Errorf("author.name = %q, expected to contain %q", embed.Author.Name, tc.wantSubs)
			}
		})
	}
}

func TestPortraitPath(t *testing.T) {
	cases := []struct {
		model string
		want  string
	}{
		{"sarge", "/assets/portraits/sarge/icon_default.png"},
		{"sarge/krusade", "/assets/portraits/sarge/icon_krusade.png"},
		{"*james", "/assets/portraits/james/icon_default.png"},
		{"*Callisto/blue", "/assets/portraits/callisto/icon_blue.png"},
		{"doom/default", "/assets/portraits/doom/icon_default.png"},
		{"", ""},
		{"/", ""}, // empty head, so no portrait
	}
	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			if got := portraitPath(tc.model); got != tc.want {
				t.Errorf("portraitPath(%q) = %q, want %q", tc.model, got, tc.want)
			}
		})
	}
}
