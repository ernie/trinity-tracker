package natsbus_test

import (
	"context"
	"fmt"
	"net"
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

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func TestLoopbackEventRoundTrip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	port := freePort(t)
	trackerCfg := &config.TrackerConfig{
		NATS: config.NATSConfig{URL: fmt.Sprintf("nats://127.0.0.1:%d", port)},
		Hub: &config.HubConfig{
			DedupWindow: config.Duration(5 * time.Minute),
			Retention:   config.Duration(24 * time.Hour),
		},
	}
	ns, err := natsbus.Start(trackerCfg, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("natsbus.Start: %v", err)
	}

	store, err := storage.New(filepath.Join(t.TempDir(), "trinity.db"))
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	srv := &domain.Server{Key: "ffa", Address: "127.0.0.1:27960"}
	if err := store.UpsertServer(ctx, "test", srv); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	if srv.ID != 1 {
		t.Fatalf("expected seeded server id=1, got %d", srv.ID)
	}

	// The source is approved up front so events dispatch at once.
	writer := hub.NewWriter(store)
	writer.Start(ctx)
	const source = "local-1"
	writer.MarkSourceApproved(source)

	subNC, err := ns.ConnectInternal(nats.Name("test-sub"))
	if err != nil {
		t.Fatalf("sub connect: %v", err)
	}
	sub, err := natsbus.NewSubscriber(subNC, writer)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	sub.Start(ctx)

	pubNC, err := ns.ConnectInternal(nats.Name("test-pub"))
	if err != nil {
		t.Fatalf("pub connect: %v", err)
	}

	// Cleanups run LIFO, so teardown goes bottom to top: stop the
	// subscriber, cancel the writer's loops, drain the writer, then
	// close the connections and the embedded server.
	t.Cleanup(func() { ns.Stop() })
	t.Cleanup(func() { _ = store.Close() })
	t.Cleanup(func() { pubNC.Close() })
	t.Cleanup(func() { subNC.Close() })
	t.Cleanup(func() { writer.Stop() })
	t.Cleanup(cancel)
	t.Cleanup(sub.Stop)
	pub, err := natsbus.NewPublisher(pubNC, source, 0)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	matchUUID := "match-loopback-1"
	ts := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	if err := pub.Publish(domain.FactEvent{
		Type:      domain.FactMatchStart,
		ServerID:  srv.ID,
		Timestamp: ts,
		Data: domain.MatchStartData{
			MatchUUID:         matchUUID,
			MapName:           "q3dm17",
			GameType:          "FFA",
			StartedAt:         ts,
			HandshakeRequired: true,
		},
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Wait up to 3s for consumed_seq to advance.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		prog, err := store.GetSourceProgress(ctx, source)
		if err != nil {
			t.Fatalf("GetSourceProgress: %v", err)
		}
		if prog.ConsumedSeq == 1 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if prog, _ := store.GetSourceProgress(ctx, source); prog.ConsumedSeq != 1 {
		t.Fatalf("consumed_seq = %d, want 1 (subscriber did not process)", prog.ConsumedSeq)
	}

	match, err := store.GetMatchByUUID(ctx, matchUUID)
	if err != nil {
		t.Fatalf("GetMatchByUUID: %v", err)
	}
	if match == nil {
		t.Fatal("match not persisted")
	}
	if match.MapName != "q3dm17" {
		t.Errorf("MapName = %q", match.MapName)
	}
	if match.ServerID != srv.ID {
		t.Errorf("ServerID = %d, want %d", match.ServerID, srv.ID)
	}

	// A second publish gets seq 2 and is processed, so the pipeline
	// keeps moving.
	if err := pub.Publish(domain.FactEvent{
		Type:      domain.FactMatchStart,
		ServerID:  srv.ID,
		Timestamp: ts,
		Data: domain.MatchStartData{
			MatchUUID:         matchUUID + "-dup",
			MapName:           "should-not-persist",
			GameType:          "FFA",
			StartedAt:         ts,
			HandshakeRequired: true,
		},
	}); err != nil {
		t.Fatalf("second publish: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if prog, _ := store.GetSourceProgress(ctx, source); prog.ConsumedSeq != 2 {
		t.Errorf("consumed_seq after second publish = %d, want 2", prog.ConsumedSeq)
	}
}
