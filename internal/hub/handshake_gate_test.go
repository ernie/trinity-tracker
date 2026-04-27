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

	srv := &domain.Server{Key: "ffa", Address: "127.0.0.1:27960"}
	if err := store.UpsertServer(ctx, "test", srv); err != nil {
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

	srv := &domain.Server{Key: "ctf", Address: "127.0.0.1:27961"}
	if err := store.UpsertServer(ctx, "test", srv); err != nil {
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

// TestIsHandshakeEnforcedLatch covers the column behind Router.Broadcast's
// gate: it must report false until match_start latches it true, and must
// flip back to false on a subsequent match_start with handshake_required=false
// (operator downgrade detected on the next match observed by the hub).
func TestIsHandshakeEnforcedLatch(t *testing.T) {
	w, store := newTestWriter(t)
	ctx := context.Background()
	const source = "aaaa-0000-0000-0000-000000000201"
	w.MarkSourceApproved(source)

	srv := &domain.Server{Key: "ffa", Address: "127.0.0.1:27960"}
	if err := store.UpsertServer(ctx, "test", srv); err != nil {
		t.Fatalf("UpsertServer: %v", err)
	}
	if err := store.TagLocalServerSource(ctx, srv.ID, source, srv.ID); err != nil {
		t.Fatalf("TagLocalServerSource: %v", err)
	}

	if w.IsHandshakeEnforced(ctx, srv.ID) {
		t.Fatal("expected IsHandshakeEnforced=false on a freshly-provisioned server")
	}

	openEnv := envelopeFor(t, source, 1, domain.FactMatchStart, domain.MatchStartData{
		MatchUUID:         "match-201",
		MapName:           "q3dm17",
		GameType:          "FFA",
		StartedAt:         time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC),
		HandshakeRequired: true,
	})
	openEnv.RemoteServerID = srv.ID
	if err := w.HandleEnvelope(ctx, openEnv); err != nil {
		t.Fatalf("HandleEnvelope(open): %v", err)
	}
	if !w.IsHandshakeEnforced(ctx, srv.ID) {
		t.Fatal("expected IsHandshakeEnforced=true after match_start with handshake_required=true")
	}

	closeEnv := envelopeFor(t, source, 2, domain.FactMatchStart, domain.MatchStartData{
		MatchUUID:         "match-201b",
		MapName:           "q3dm17",
		GameType:          "FFA",
		StartedAt:         time.Date(2026, 4, 19, 12, 30, 0, 0, time.UTC),
		HandshakeRequired: false,
	})
	closeEnv.RemoteServerID = srv.ID
	if err := w.HandleEnvelope(ctx, closeEnv); err != nil {
		t.Fatalf("HandleEnvelope(close): %v", err)
	}
	if w.IsHandshakeEnforced(ctx, srv.ID) {
		t.Fatal("expected IsHandshakeEnforced=false after match_start with handshake_required=false")
	}
}
