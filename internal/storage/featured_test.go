package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
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

func TestGetFilteredMatchSummaries_FeaturedAndDemoFilters(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	featuredWithDemo := seedFinishedMatch(t, store, true)
	featuredNoDemo := seedFinishedMatch(t, store, false)
	plainWithDemo := seedFinishedMatch(t, store, true)

	for _, id := range []int64{featuredWithDemo, featuredNoDemo} {
		if err := store.SetMatchFeatured(ctx, id, true); err != nil {
			t.Fatal(err)
		}
	}

	idsOf := func(ms []domain.MatchSummary) map[int64]bool {
		out := make(map[int64]bool, len(ms))
		for _, m := range ms {
			out[m.ID] = true
		}
		return out
	}

	// FeaturedOnly includes flagged matches regardless of demo state — the
	// admin view surfaces demo-less entries that GetFeaturedMatches hides.
	got, err := store.GetFilteredMatchSummaries(ctx, MatchFilter{FeaturedOnly: true, IncludeBotOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	ids := idsOf(got)
	if len(ids) != 2 || !ids[featuredWithDemo] || !ids[featuredNoDemo] {
		t.Fatalf("FeaturedOnly: expected {%d %d}, got %v", featuredWithDemo, featuredNoDemo, ids)
	}

	got, err = store.GetFilteredMatchSummaries(ctx, MatchFilter{DemoOnly: true, IncludeBotOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	ids = idsOf(got)
	if len(ids) != 2 || !ids[featuredWithDemo] || !ids[plainWithDemo] {
		t.Fatalf("DemoOnly: expected {%d %d}, got %v", featuredWithDemo, plainWithDemo, ids)
	}

	got, err = store.GetFilteredMatchSummaries(ctx, MatchFilter{FeaturedOnly: true, DemoOnly: true, IncludeBotOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	ids = idsOf(got)
	if len(ids) != 1 || !ids[featuredWithDemo] {
		t.Fatalf("FeaturedOnly+DemoOnly: expected {%d}, got %v", featuredWithDemo, ids)
	}
}

// seedFinishedMatch builds on seedTestMatch with what
// GetFilteredMatchSummaries additionally requires: a non-null ended_at and
// one stats row to satisfy the match_player_stats join.
func seedFinishedMatch(t *testing.T, store *Store, hasDemo bool) int64 {
	t.Helper()
	ctx := context.Background()
	matchID := seedTestMatch(t, store, true, hasDemo)

	if _, err := store.db.ExecContext(ctx,
		`UPDATE matches SET started_at = '2026-01-01T12:00:00Z', ended_at = '2026-01-01T12:10:00Z' WHERE id = ?`,
		matchID); err != nil {
		t.Fatal(err)
	}

	res, err := store.db.ExecContext(ctx,
		`INSERT INTO players (name, clean_name) VALUES ('tester', 'tester')`)
	if err != nil {
		t.Fatal(err)
	}
	playerID, _ := res.LastInsertId()
	res, err = store.db.ExecContext(ctx,
		`INSERT INTO player_guids (player_id, guid, name, clean_name) VALUES (?, ?, 'tester', 'tester')`,
		playerID, "GUID-"+randomSuffix())
	if err != nil {
		t.Fatal(err)
	}
	guidID, _ := res.LastInsertId()
	if _, err := store.db.ExecContext(ctx,
		`INSERT INTO match_player_stats (match_id, player_guid_id) VALUES (?, ?)`,
		matchID, guidID); err != nil {
		t.Fatal(err)
	}
	return matchID
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
