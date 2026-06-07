package storage

import (
	"context"
	"slices"
	"testing"

	"github.com/ernie/trinity-tracker/internal/domain"
)

func seedPlayer(t *testing.T, store *Store, name, firstSeen, lastSeen string, isBot bool) int64 {
	t.Helper()
	res, err := store.db.ExecContext(context.Background(),
		`INSERT INTO players (name, clean_name, first_seen, last_seen, is_bot) VALUES (?, ?, ?, ?, ?)`,
		name, name, firstSeen, lastSeen, isBot)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestGetPlayers_SortAndBotFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	alice := seedPlayer(t, store, "alice", "2026-01-03T00:00:00Z", "2026-01-10T00:00:00Z", false)
	zed := seedPlayer(t, store, "zed", "2026-01-01T00:00:00Z", "2026-01-20T00:00:00Z", false)
	bot := seedPlayer(t, store, "sarge", "2026-01-02T00:00:00Z", "2026-01-30T00:00:00Z", true)

	ids := func(ps []domain.Player) []int64 {
		out := make([]int64, len(ps))
		for i, p := range ps {
			out[i] = p.ID
		}
		return out
	}

	// Default listing: humans only, and total reflects the filter.
	players, total, err := store.GetPlayers(ctx, PlayerListOptions{Sort: "last_seen", Desc: true})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || !slices.Equal(ids(players), []int64{zed, alice}) {
		t.Fatalf("last_seen desc humans: total=%d ids=%v", total, ids(players))
	}

	players, total, err = store.GetPlayers(ctx, PlayerListOptions{Sort: "last_seen", Desc: true, IncludeBots: true})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || !slices.Equal(ids(players), []int64{bot, zed, alice}) {
		t.Fatalf("last_seen desc with bots: total=%d ids=%v", total, ids(players))
	}

	players, _, err = store.GetPlayers(ctx, PlayerListOptions{Sort: "name"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(players), []int64{alice, zed}) {
		t.Fatalf("name asc: ids=%v", ids(players))
	}

	players, _, err = store.GetPlayers(ctx, PlayerListOptions{Sort: "first_seen"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(players), []int64{zed, alice}) {
		t.Fatalf("first_seen asc: ids=%v", ids(players))
	}

	// Unknown keys fall back to last_seen rather than reaching SQL.
	players, _, err = store.GetPlayers(ctx, PlayerListOptions{Sort: "evil; DROP TABLE players", Desc: true})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(players), []int64{zed, alice}) {
		t.Fatalf("fallback sort: ids=%v", ids(players))
	}

	// Offset pagination keeps the filtered total.
	players, total, err = store.GetPlayers(ctx, PlayerListOptions{Limit: 1, Offset: 1, Sort: "last_seen", Desc: true})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || !slices.Equal(ids(players), []int64{alice}) {
		t.Fatalf("offset page: total=%d ids=%v", total, ids(players))
	}
}
