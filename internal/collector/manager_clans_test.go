package collector

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

func matchStartFacts(t *testing.T, settings map[string]string) []domain.MatchStartData {
	t.Helper()
	state := &serverState{server: domain.Server{ID: 7}, clients: map[int]*clientState{}}
	pub := &vrTestPublisher{}
	m := &ServerManager{
		events:  make(chan domain.Event, 8),
		servers: map[int64]*serverState{7: state},
		server:  vrTestServer{},
		pub:     pub,
	}
	settings["g_trinityhandshake"] = "1"
	start := time.Date(2026, 11, 22, 20, 0, 0, 0, time.UTC)
	ctx := context.Background()
	m.handleLogEvent(ctx, 7, LogEvent{Timestamp: start, Type: EventTypeInitGame, Data: InitGameData{
		MapName: "mpteam1", GameType: 4, UUID: "clan-match", Settings: settings,
	}}, false)
	m.handleLogEvent(ctx, 7, LogEvent{Timestamp: start.Add(time.Second), Type: EventTypeWarmupEnd}, false)

	var got []domain.MatchStartData
	for _, f := range pub.facts {
		if f.Type == domain.FactMatchStart {
			got = append(got, f.Data.(domain.MatchStartData))
		}
	}
	if len(got) != 1 {
		t.Fatalf("match_start facts = %d, want 1", len(got))
	}
	return got
}

func TestMatchStartCarriesClanNames(t *testing.T) {
	got := matchStartFacts(t, map[string]string{"g_redteam": "The Flock", "g_blueteam": "Team Beef"})[0]
	if got.RedTeam != "The Flock" || got.BlueTeam != "Team Beef" {
		t.Fatalf("teams = %q, %q; want The Flock, Team Beef", got.RedTeam, got.BlueTeam)
	}
}

func TestMatchStartWithoutClanNames(t *testing.T) {
	got := matchStartFacts(t, map[string]string{})[0]
	if got.RedTeam != "" || got.BlueTeam != "" {
		t.Fatalf("teams = %q, %q; want empty", got.RedTeam, got.BlueTeam)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "red_team") || strings.Contains(string(raw), "blue_team") {
		t.Fatalf("json = %s, want no team keys", raw)
	}
}
