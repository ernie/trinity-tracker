package natsbus_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/config"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/hub"
	"github.com/ernie/trinity-tracker/internal/natsbus"
	"github.com/ernie/trinity-tracker/internal/storage"
)

// TestTwoProcessMatchLifecycle exercises the collector-only
// deployment path from the outside: a hub boots with all its
// subscriptions, provisions a new source through the admin-ish code
// path (CreateSource + MintUserCreds), hands the issued .creds file
// to a separate NATS connection opened over TCP — not the
// nats.InProcessServer shortcut — and then drives a scripted match
// lifecycle through the same RPCs + publishers the real collector
// uses. At the end we read the hub's DB and assert every expected
// row landed.
func TestTwoProcessMatchLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	hubDataDir := t.TempDir()
	port := freePort(t)
	cfg := &config.TrackerConfig{
		NATS: config.NATSConfig{URL: fmt.Sprintf("nats://127.0.0.1:%d", port)},
		Hub: &config.HubConfig{
			DedupWindow: config.Duration(5 * time.Minute),
			Retention:   config.Duration(24 * time.Hour),
		},
	}
	ns, err := natsbus.Start(cfg, hubDataDir)
	if err != nil {
		t.Fatalf("natsbus.Start: %v", err)
	}

	store, err := storage.New(filepath.Join(t.TempDir(), "trinity.db"))
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}

	writer := hub.NewWriter(store)
	writer.Start(ctx)

	subNC, err := ns.ConnectInternal(nats.Name("hub-sub"))
	if err != nil {
		t.Fatalf("hub sub connect: %v", err)
	}
	sub, err := natsbus.NewSubscriber(subNC, writer)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	sub.Start(ctx)

	rpcSrv, err := natsbus.RegisterRPCHandlers(subNC, writer)
	if err != nil {
		t.Fatalf("RegisterRPCHandlers: %v", err)
	}

	// Teardown in LIFO order: stop inbound first, then cancel the
	// writer's ctx, then stop the writer (so its linkCode loop
	// unblocks), then close conns + NATS.
	t.Cleanup(func() { ns.Stop() })
	t.Cleanup(func() { _ = store.Close() })
	t.Cleanup(func() { subNC.Close() })
	t.Cleanup(func() { writer.Stop() })
	t.Cleanup(cancel)
	t.Cleanup(sub.Stop)
	t.Cleanup(rpcSrv.Stop)

	// --- Provisioning (what the admin UI's POST /api/admin/sources does) ---
	const source = "remote-1"
	const localID = int64(42)
	reg := domain.Registration{
		Source:  source,
		Version: "1.0.0-test",
		Servers: []domain.RegdServer{
			{LocalID: localID, Key: "ffa", Address: "127.0.0.1:27960"},
		},
	}
	if err := store.CreateSource(ctx, source, true); err != nil {
		t.Fatalf("CreateSource: %v", err)
	}
	writer.MarkSourceApproved(source)
	if _, err := ns.Auth().MintUserCreds(source); err != nil {
		t.Fatalf("MintUserCreds: %v", err)
	}
	credsPath := ns.Auth().CredsPath(source)
	if err := store.UpsertRemoteServers(ctx, reg); err != nil {
		t.Fatalf("UpsertRemoteServers: %v", err)
	}

	// --- Collector-side connection over real TCP with issued creds ---
	collectorNC, err := nats.Connect(ns.ClientURL(),
		nats.UserCredentials(credsPath),
		nats.CustomInboxPrefix(natsbus.InboxPrefixFor(source)),
		nats.Name("remote-collector"),
	)
	if err != nil {
		t.Fatalf("collector connect: %v", err)
	}
	t.Cleanup(collectorNC.Close)

	pub, err := natsbus.NewPublisher(collectorNC, source, 0)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	rpcClient, err := natsbus.NewRPCClient(collectorNC, source, 2*time.Second)
	if err != nil {
		t.Fatalf("NewRPCClient: %v", err)
	}

	// --- Scripted match lifecycle ---
	now := time.Date(2026, 4, 19, 16, 0, 0, 0, time.UTC)

	// 1. Upsert identity for the human player via RPC (what the
	//    collector does at ClientUserinfo time).
	const (
		humanGUID  = "GUIDHUMANAAAAAAAAAAAAAAAAAAAAAAA"
		humanName  = "Alice"
		humanClean = "alice"
	)
	id, err := rpcClient.UpsertPlayerIdentity(ctx, humanGUID, humanName, humanClean, now, false)
	if err != nil {
		t.Fatalf("UpsertPlayerIdentity: %v", err)
	}
	if !id.Found || id.PlayerID == 0 {
		t.Fatalf("identity not persisted: %+v", id)
	}

	// 2. Match start.
	matchUUID := "MATCH-TWO-PROCESS-LIFECYCLE"
	if err := pub.Publish(domain.FactEvent{
		Type:      domain.FactMatchStart,
		ServerID:  localID,
		Timestamp: now,
		Data: domain.MatchStartData{
			MatchUUID:         matchUUID,
			MapName:           "q3dm17",
			GameType:          "FFA",
			StartedAt:         now,
			HandshakeRequired: true,
		},
	}); err != nil {
		t.Fatalf("match_start Publish: %v", err)
	}

	// 3. Player join.
	joinedAt := now.Add(5 * time.Second)
	if err := pub.Publish(domain.FactEvent{
		Type:      domain.FactPlayerJoin,
		ServerID:  localID,
		Timestamp: joinedAt,
		Data: domain.PlayerJoinData{
			MatchUUID: matchUUID,
			GUID:      humanGUID,
			Name:      humanName,
			CleanName: humanClean,
			IP:        "10.0.0.2",
			IsBot:     false,
			IsVR:      false,
			JoinedAt:  joinedAt,
			ClientNum: 3,
		},
	}); err != nil {
		t.Fatalf("player_join Publish: %v", err)
	}

	// 4. Trinity handshake stamps client_engine/client_version on
	//    the open session.
	handshakeAt := joinedAt.Add(1 * time.Second)
	if err := pub.Publish(domain.FactEvent{
		Type:      domain.FactTrinityHandshake,
		ServerID:  localID,
		Timestamp: handshakeAt,
		Data: domain.TrinityHandshakeData{
			GUID:          humanGUID,
			ClientEngine:  "Trinity",
			ClientVersion: "1.0.0",
		},
	}); err != nil {
		t.Fatalf("trinity_handshake Publish: %v", err)
	}

	// 5. Match end with one stats row.
	endedAt := joinedAt.Add(10 * time.Minute)
	redScore := 20
	blueScore := 12
	if err := pub.Publish(domain.FactEvent{
		Type:      domain.FactMatchEnd,
		ServerID:  localID,
		Timestamp: endedAt,
		Data: domain.MatchEndData{
			MatchUUID:  matchUUID,
			EndedAt:    endedAt,
			ExitReason: "fraglimit",
			RedScore:   &redScore,
			BlueScore:  &blueScore,
			Players: []domain.MatchEndPlayer{
				{
					GUID:      humanGUID,
					ClientID:  3,
					Name:      humanName,
					CleanName: humanClean,
					Frags:     20,
					Deaths:    12,
					Completed: true,
					Victory:   true,
					JoinedAt:  joinedAt,
				},
			},
		},
	}); err != nil {
		t.Fatalf("match_end Publish: %v", err)
	}

	// --- Wait for consumed_seq to catch up, then assert DB state ---
	deadline := time.Now().Add(3 * time.Second)
	var consumed uint64
	for time.Now().Before(deadline) {
		prog, _ := store.GetSourceProgress(ctx, source)
		consumed = prog.ConsumedSeq
		if consumed >= 4 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if consumed < 4 {
		t.Fatalf("consumed_seq = %d after 3s, want ≥ 4", consumed)
	}

	match, err := store.GetMatchByUUID(ctx, matchUUID)
	if err != nil || match == nil {
		t.Fatalf("match not persisted: match=%v err=%v", match, err)
	}
	if match.EndedAt == nil {
		t.Error("match ended_at not set")
	}
	if match.MapName != "q3dm17" {
		t.Errorf("MapName = %q", match.MapName)
	}

	// match_player_stats row should exist for Alice.
	pg, err := store.GetPlayerGUIDByGUID(ctx, humanGUID)
	if err != nil || pg == nil {
		t.Fatalf("player_guids row missing for %s: %v", humanGUID, err)
	}
	summary, err := store.GetMatchSummaryByID(ctx, match.ID)
	if err != nil {
		t.Fatalf("GetMatchSummaryByID: %v", err)
	}
	if summary == nil || len(summary.Players) != 1 {
		t.Fatalf("match summary players = %d, want 1 (summary=%+v)", len(summary.Players), summary)
	}
	if summary.Players[0].Frags != 20 || summary.Players[0].Deaths != 12 {
		t.Errorf("player stats = %+v", summary.Players[0])
	}
}
