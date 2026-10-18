package storage

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedMergePlayer(t *testing.T, s *Store, guid string) int64 {
	t.Helper()
	pg, err := s.UpsertPlayerGUID(context.Background(), guid, guid, guid, time.Now().UTC(), false)
	if err != nil {
		t.Fatal(err)
	}
	return pg.PlayerID
}

func linkedPlayerID(t *testing.T, s *Store, username string) *int64 {
	t.Helper()
	u, err := s.GetUserByUsername(context.Background(), username)
	if err != nil {
		t.Fatal(err)
	}
	return u.PlayerID
}

func TestMergePlayersMovesAccountLinkToTarget(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	target := seedMergePlayer(t, s, "GUID-TARGET")
	source := seedMergePlayer(t, s, "GUID-SOURCE")
	if err := s.CreateUser(ctx, "alice", "hash", false, &source); err != nil {
		t.Fatal(err)
	}

	if err := s.MergePlayers(ctx, target, source); err != nil {
		t.Fatalf("MergePlayers: %v", err)
	}

	got := linkedPlayerID(t, s, "alice")
	if got == nil || *got != target {
		t.Fatalf("alice.player_id = %v, want %d", got, target)
	}
}

func TestClaimLinkMergesClaimedPlayerIntoUsersPlayer(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	own := seedMergePlayer(t, s, "GUID-OWN")
	claimed := seedMergePlayer(t, s, "GUID-CLAIMED")
	if err := s.CreateUser(ctx, "alice", "hash", false, &own); err != nil {
		t.Fatal(err)
	}
	u, err := s.GetUserByUsername(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	code, err := s.CreateClaimCode(ctx, claimed, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	if err := s.ClaimLink(ctx, code.ID, claimed, u.ID); err != nil {
		t.Fatalf("ClaimLink: %v", err)
	}

	pg, err := s.GetPlayerGUIDByGUID(ctx, "GUID-CLAIMED")
	if err != nil {
		t.Fatal(err)
	}
	if pg.PlayerID != own {
		t.Fatalf("claimed GUID on player %d, want %d", pg.PlayerID, own)
	}
	if got := linkedPlayerID(t, s, "alice"); got == nil || *got != own {
		t.Fatalf("alice.player_id = %v, want %d", got, own)
	}
}

func TestMergePlayersRefusesWhenBothLinked(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	target := seedMergePlayer(t, s, "GUID-TARGET")
	source := seedMergePlayer(t, s, "GUID-SOURCE")
	if err := s.CreateUser(ctx, "alice", "hash", false, &target); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateUser(ctx, "bob", "hash", false, &source); err != nil {
		t.Fatal(err)
	}

	err := s.MergePlayers(ctx, target, source)
	if !errors.Is(err, ErrBothPlayersLinked) {
		t.Fatalf("MergePlayers err = %v, want ErrBothPlayersLinked", err)
	}

	if got := linkedPlayerID(t, s, "bob"); got == nil || *got != source {
		t.Fatalf("bob.player_id = %v, want %d", got, source)
	}
	pg, err := s.GetPlayerGUIDByGUID(ctx, "GUID-SOURCE")
	if err != nil {
		t.Fatal(err)
	}
	if pg.PlayerID != source {
		t.Fatalf("source GUID moved to player %d, want %d", pg.PlayerID, source)
	}
}
