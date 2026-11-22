package hub

import (
	"context"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// startMatch sends each match_start for one match and returns its stored summary.
func startMatch(t *testing.T, starts ...domain.MatchStartData) *domain.MatchSummary {
	t.Helper()
	w, store := newTestWriter(t)
	ctx := context.Background()
	source := "aaaa-0000-0000-0000-000000000301"
	w.MarkSourceApproved(source)

	srv := &domain.Server{Key: "ta", Address: "127.0.0.1:27962"}
	if err := store.UpsertServer(ctx, "test", srv); err != nil {
		t.Fatalf("UpsertServer: %v", err)
	}
	if err := store.TagLocalServerSource(ctx, srv.ID, source, srv.ID); err != nil {
		t.Fatalf("TagLocalServerSource: %v", err)
	}
	for i, data := range starts {
		data.MatchUUID = "clan-match"
		data.MapName = "mpteam1"
		data.GameType = "tdm"
		data.StartedAt = time.Date(2026, 11, 22, 20, 0, 0, 0, time.UTC)
		data.HandshakeRequired = true
		env := envelopeFor(t, source, uint64(i+1), domain.FactMatchStart, data)
		env.RemoteServerID = srv.ID
		if err := w.HandleEnvelope(ctx, env); err != nil {
			t.Fatalf("HandleEnvelope: %v", err)
		}
	}

	m, err := store.GetMatchByUUID(ctx, "clan-match")
	if err != nil || m == nil {
		t.Fatalf("GetMatchByUUID = %v, %v", m, err)
	}
	summary, err := store.GetMatchSummaryByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetMatchSummaryByID: %v", err)
	}
	return summary
}

func TestMatchStartStoresClanNames(t *testing.T) {
	got := startMatch(t, domain.MatchStartData{RedTeam: "The Flock", BlueTeam: "Team Beef"})
	if got.RedTeam != "The Flock" || got.BlueTeam != "Team Beef" {
		t.Fatalf("teams = %q, %q; want The Flock, Team Beef", got.RedTeam, got.BlueTeam)
	}
}

func TestMatchStartWithoutClanNamesStoresEmpty(t *testing.T) {
	got := startMatch(t, domain.MatchStartData{})
	if got.RedTeam != "" || got.BlueTeam != "" {
		t.Fatalf("teams = %q, %q; want empty", got.RedTeam, got.BlueTeam)
	}
}

func TestAdoptedMatchKeepsClanNames(t *testing.T) {
	got := startMatch(t, domain.MatchStartData{RedTeam: "The Flock", BlueTeam: "Team Beef"},
		domain.MatchStartData{RedTeam: "Pagans", BlueTeam: "Stroggs"})
	if got.RedTeam != "The Flock" || got.BlueTeam != "Team Beef" {
		t.Fatalf("teams = %q, %q; want The Flock, Team Beef", got.RedTeam, got.BlueTeam)
	}
}
