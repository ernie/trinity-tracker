package storage

import (
	"context"
	"testing"
)

func TestSetUserPortrait_RoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.CreateUser(ctx, "bob", "hash", false, nil); err != nil {
		t.Fatal(err)
	}
	user, err := store.GetUserByUsername(ctx, "bob")
	if err != nil {
		t.Fatal(err)
	}
	if user.Portrait != "" {
		t.Fatalf("new user portrait = %q, want empty", user.Portrait)
	}

	if err := store.SetUserPortrait(ctx, user.ID, "sarge/krusade"); err != nil {
		t.Fatal(err)
	}
	user, err = store.GetUserByUsername(ctx, "bob")
	if err != nil {
		t.Fatal(err)
	}
	if user.Portrait != "sarge/krusade" {
		t.Fatalf("portrait = %q, want sarge/krusade", user.Portrait)
	}

	if err := store.SetUserPortrait(ctx, user.ID, ""); err != nil {
		t.Fatal(err)
	}
	user, err = store.GetUserByUsername(ctx, "bob")
	if err != nil {
		t.Fatal(err)
	}
	if user.Portrait != "" {
		t.Fatalf("cleared portrait = %q, want empty", user.Portrait)
	}
}

func seedRecentModel(t *testing.T, store *Store, playerID int64, model string) {
	t.Helper()
	ctx := context.Background()
	guidRes, err := store.db.ExecContext(ctx,
		`INSERT INTO player_guids (player_id, guid, name, clean_name, first_seen, last_seen)
		 VALUES (?, ?, 'x', 'x', '2026-01-01T00:00:00Z', '2026-01-02T00:00:00Z')`,
		playerID, "GUID-"+t.Name())
	if err != nil {
		t.Fatal(err)
	}
	guidID, _ := guidRes.LastInsertId()
	srvRes, err := store.db.ExecContext(ctx,
		`INSERT INTO servers (key, address, source) VALUES (?, 'localhost:0', 'local')`,
		"srv-"+t.Name())
	if err != nil {
		t.Fatal(err)
	}
	serverID, _ := srvRes.LastInsertId()
	matchRes, err := store.db.ExecContext(ctx,
		`INSERT INTO matches (uuid, server_id, ended_at) VALUES (?, ?, '2026-01-01T01:00:00Z')`,
		"match-"+t.Name(), serverID)
	if err != nil {
		t.Fatal(err)
	}
	matchID, _ := matchRes.LastInsertId()
	if _, err := store.db.ExecContext(ctx,
		`INSERT INTO match_player_stats (match_id, player_guid_id, model) VALUES (?, ?, ?)`,
		matchID, guidID, model); err != nil {
		t.Fatal(err)
	}
}

func TestPortraitOverridesRecentModel(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	pid := seedPlayer(t, store, "alice", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z", false)
	seedRecentModel(t, store, pid, "sarge/default")
	if err := store.CreateUser(ctx, "alice", "hash", false, &pid); err != nil {
		t.Fatal(err)
	}
	user, err := store.GetUserByUsername(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}

	p, err := store.GetPlayerByID(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if p.Model != "sarge/default" {
		t.Fatalf("unset portrait: model = %q, want sarge/default", p.Model)
	}

	if err := store.SetUserPortrait(ctx, user.ID, "xaero/blue"); err != nil {
		t.Fatal(err)
	}
	p, err = store.GetPlayerByID(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if p.Model != "xaero/blue" {
		t.Fatalf("set portrait: model = %q, want xaero/blue", p.Model)
	}

	results, err := store.SearchPlayers(ctx, "alice", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Model != "xaero/blue" {
		t.Fatalf("search: results = %+v, want one with model xaero/blue", results)
	}

	if err := store.SetUserPortrait(ctx, user.ID, ""); err != nil {
		t.Fatal(err)
	}
	p, err = store.GetPlayerByID(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if p.Model != "sarge/default" {
		t.Fatalf("cleared portrait: model = %q, want sarge/default", p.Model)
	}
}
