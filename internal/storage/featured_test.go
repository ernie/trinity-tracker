package storage

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestSetMatchFeatured(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Seed: insert a match with has_human_player=1 and demo_available=1
	matchID := seedTestMatch(t, store, true, true)

	// Initially not featured
	ids, err := store.GetFeaturedMatches(ctx, 20)
	if err != nil {
		t.Fatalf("GetFeaturedMatches: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 featured, got %d", len(ids))
	}

	// Set featured
	if err := store.SetMatchFeatured(ctx, matchID, true); err != nil {
		t.Fatalf("SetMatchFeatured: %v", err)
	}

	ids, err = store.GetFeaturedMatches(ctx, 20)
	if err != nil {
		t.Fatalf("GetFeaturedMatches: %v", err)
	}
	if len(ids) != 1 || ids[0] != matchID {
		t.Fatalf("expected [%d], got %v", matchID, ids)
	}

	// Unset
	if err := store.SetMatchFeatured(ctx, matchID, false); err != nil {
		t.Fatalf("SetMatchFeatured(false): %v", err)
	}
	ids, _ = store.GetFeaturedMatches(ctx, 20)
	if len(ids) != 0 {
		t.Fatalf("expected 0 after unset, got %d", len(ids))
	}
}

func TestGetFeaturedMatches_ExcludesNoDemoMatches(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	withDemo := seedTestMatch(t, store, true, true)
	noDemoID := seedTestMatch(t, store, true, false) // has human, no demo

	if err := store.SetMatchFeatured(ctx, withDemo, true); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMatchFeatured(ctx, noDemoID, true); err != nil {
		t.Fatal(err)
	}

	ids, err := store.GetFeaturedMatches(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != withDemo {
		t.Fatalf("only the with-demo match should appear; got %v", ids)
	}
}

// seedTestMatch inserts a minimal valid match row for tests.
func seedTestMatch(t *testing.T, store *Store, hasHuman, hasDemo bool) int64 {
	t.Helper()
	ctx := context.Background()
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO servers (key, address, source) VALUES (?, ?, ?)
		ON CONFLICT DO NOTHING
	`, "test-srv", "127.0.0.1:27960", "test")
	if err != nil {
		t.Fatal(err)
	}
	var serverID int64
	if err := store.db.QueryRowContext(ctx, "SELECT id FROM servers WHERE source='test' AND key='test-srv'").Scan(&serverID); err != nil {
		t.Fatal(err)
	}

	humanInt := 0
	if hasHuman {
		humanInt = 1
	}
	demoInt := 0
	if hasDemo {
		demoInt = 1
	}
	res, err := store.db.ExecContext(ctx, `
		INSERT INTO matches (uuid, server_id, map_name, game_type, has_human_player, demo_available)
		VALUES (?, ?, 'q3dm17', 'ffa', ?, ?)
	`, t.Name()+"-"+randomSuffix(), serverID, humanInt, demoInt)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// suffixCounter increments per call to guarantee unique uuids even within a
// single nanosecond. UnixNano can't be relied on alone — two consecutive
// inserts can land in the same instant on fast machines.
var suffixCounter int64

func randomSuffix() string {
	suffixCounter++
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), suffixCounter)
}
