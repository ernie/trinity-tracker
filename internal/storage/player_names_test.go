package storage

import (
	"context"
	"testing"
	"time"
)

// akaNames returns the set of clean names GetPlayerNames reports for a player.
func akaNames(t *testing.T, s *Store, playerID int64) map[string]bool {
	t.Helper()
	names, err := s.GetPlayerNames(context.Background(), playerID)
	if err != nil {
		t.Fatalf("GetPlayerNames: %v", err)
	}
	got := make(map[string]bool, len(names))
	for _, pn := range names {
		got[pn.CleanName] = true
	}
	return got
}

// The alias history is the source for the "Also known as" list. A human only
// lands on a bot name or the engine default UnnamedPlayer transiently (e.g. by
// trying to use a reserved name), so those must never be recorded against them.
func TestUpsertPlayerGUIDSkipsReservedNames(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	const guid = "HUMANGUID"
	used := []string{"RealName", "Xaero", "UnnamedPlayer", "AnotherReal"}
	var playerID int64
	for i, n := range used {
		pg, err := s.UpsertPlayerGUID(ctx, guid, n, n, base.Add(time.Duration(i)*time.Minute), false)
		if err != nil {
			t.Fatalf("UpsertPlayerGUID(%q): %v", n, err)
		}
		playerID = pg.PlayerID
	}

	got := akaNames(t, s, playerID)
	for _, want := range []string{"RealName", "AnotherReal"} {
		if !got[want] {
			t.Errorf("expected %q recorded, got %v", want, mapKeys(got))
		}
	}
	for _, reserved := range []string{"Xaero", "UnnamedPlayer"} {
		if got[reserved] {
			t.Errorf("reserved name %q should not be recorded, got %v", reserved, mapKeys(got))
		}
	}
}

// A display name locked by one user must not be recorded as an alias for a
// different player who briefly used it (the server forces them off it), but the
// owner is free to keep recording it.
func TestUpsertPlayerGUIDSkipsNamesOwnedByAnotherUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Owner: a player with an account whose locked display name is "Locked".
	owner, err := s.UpsertPlayerGUID(ctx, "OWNERGUID", "Owner", "Owner", base, false)
	if err != nil {
		t.Fatalf("UpsertPlayerGUID(owner): %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, is_admin, player_id, display_name, display_name_canonical)
		VALUES (?, ?, FALSE, ?, ?, ?)
	`, "owner", "x", owner.PlayerID, "Locked", "locked"); err != nil {
		t.Fatalf("seed owner user: %v", err)
	}

	// Intruder: a different player who momentarily used "Locked".
	intruder, err := s.UpsertPlayerGUID(ctx, "INTRUDERGUID", "Locked", "Locked", base.Add(time.Minute), false)
	if err != nil {
		t.Fatalf("UpsertPlayerGUID(intruder): %v", err)
	}
	if intruder.PlayerID == owner.PlayerID {
		t.Fatal("intruder unexpectedly shares the owner's player id")
	}

	if got := akaNames(t, s, intruder.PlayerID); got["Locked"] {
		t.Errorf("name locked by another user should not be recorded for intruder, got %v", mapKeys(got))
	}

	// The owner re-using their own locked name is recorded normally.
	if _, err := s.UpsertPlayerGUID(ctx, "OWNERGUID", "Locked", "Locked", base.Add(2*time.Minute), false); err != nil {
		t.Fatalf("UpsertPlayerGUID(owner re-use): %v", err)
	}
	if got := akaNames(t, s, owner.PlayerID); !got["Locked"] {
		t.Errorf("owner should be able to record their own locked name, got %v", mapKeys(got))
	}
}

func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
