package hub

import (
	"context"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

func TestHandleMatchStartRejectsWhenHandshakeNotRequired(t *testing.T) {
	w, store := newTestWriter(t)
	ctx := context.Background()
	uuid := "aaaa-0000-0000-0000-000000000101"
	w.MarkSourceApproved(uuid)

	srv := &domain.Server{Name: "ffa", Address: "127.0.0.1:27960"}
	if err := store.UpsertServer(ctx, srv); err != nil {
		t.Fatalf("UpsertServer: %v", err)
	}
	if err := store.TagLocalServerSource(ctx, srv.ID, uuid, srv.ID); err != nil {
		t.Fatalf("TagLocalServerSource: %v", err)
	}

	env := envelopeFor(t, uuid, 1, domain.FactMatchStart, domain.MatchStartData{
		MatchUUID:         "match-no-handshake",
		MapName:           "q3dm17",
		GameType:          "FFA",
		StartedAt:         time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC),
		HandshakeRequired: false,
	})
	env.RemoteServerID = srv.ID
	if err := w.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("HandleEnvelope: %v", err)
	}

	if m, _ := store.GetMatchByUUID(ctx, "match-no-handshake"); m != nil {
		t.Errorf("match created despite handshake_required=false: %+v", m)
	}
}

func TestHandleMatchStartAcceptsWhenHandshakeRequired(t *testing.T) {
	w, store := newTestWriter(t)
	ctx := context.Background()
	uuid := "aaaa-0000-0000-0000-000000000102"
	w.MarkSourceApproved(uuid)

	srv := &domain.Server{Name: "ctf", Address: "127.0.0.1:27961"}
	if err := store.UpsertServer(ctx, srv); err != nil {
		t.Fatalf("UpsertServer: %v", err)
	}
	if err := store.TagLocalServerSource(ctx, srv.ID, uuid, srv.ID); err != nil {
		t.Fatalf("TagLocalServerSource: %v", err)
	}

	env := envelopeFor(t, uuid, 1, domain.FactMatchStart, domain.MatchStartData{
		MatchUUID:         "match-with-handshake",
		MapName:           "q3ctf1",
		GameType:          "CTF",
		StartedAt:         time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC),
		HandshakeRequired: true,
	})
	env.RemoteServerID = srv.ID
	if err := w.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("HandleEnvelope: %v", err)
	}

	m, err := store.GetMatchByUUID(ctx, "match-with-handshake")
	if err != nil {
		t.Fatalf("GetMatchByUUID: %v", err)
	}
	if m == nil {
		t.Fatal("match not persisted despite handshake_required=true")
	}
	if m.MapName != "q3ctf1" {
		t.Errorf("MapName = %q, want q3ctf1", m.MapName)
	}
}
