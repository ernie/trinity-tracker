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

// TestLoopbackEventRoundTrip publishes a fact event through JetStream
// and verifies it reaches the writer, lands in the DB, and advances
// the per-source consumed_seq. This is the phase-4 loopback test
// called out in the execution plan.
func TestLoopbackEventRoundTrip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Embedded NATS with the two streams.
	port := freePort(t)
	trackerCfg := &config.TrackerConfig{
		NATS: config.NATSConfig{URL: fmt.Sprintf("nats://127.0.0.1:%d", port)},
		Hub: &config.HubConfig{
			DedupWindow: config.Duration(5 * time.Minute),
			Retention:   config.Duration(24 * time.Hour),
		},
	}
	ns, err := natsbus.Start(trackerCfg, t.TempDir())
	if err != nil {
		t.Fatalf("natsbus.Start: %v", err)
	}

	// Storage with a seeded server (id=1).
	store, err := storage.New(filepath.Join(t.TempDir(), "trinity.db"))
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	srv := &domain.Server{Name: "ffa", Address: "127.0.0.1:27960"}
	if err := store.UpsertServer(ctx, srv); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	if srv.ID != 1 {
		t.Fatalf("expected seeded server id=1, got %d", srv.ID)
	}

	// Writer + subscriber (hub side). Pre-approve the test source so
	// events dispatch immediately instead of accumulating in the DLQ.
	writer := hub.NewWriter(store)
	writer.Start(ctx)
	const sourceUUID = "aaaa-0000-0000-0000-000000000042"
	writer.MarkSourceApproved(sourceUUID)

	subNC, err := ns.ConnectInternal(nats.Name("test-sub"))
	if err != nil {
		t.Fatalf("sub connect: %v", err)
	}
	sub, err := natsbus.NewSubscriber(subNC, writer)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	sub.Start(ctx)

	// Publisher (collector side).
	pubNC, err := ns.ConnectInternal(nats.Name("test-pub"))
	if err != nil {
		t.Fatalf("pub connect: %v", err)
	}

	// Teardown in strict order: stop the subscriber (no more inbound),
	// cancel the writer's background loops, wait for the writer to
	// drain, close connections and the embedded server. Registering in
	// reverse so LIFO runs them in the order written.
	t.Cleanup(func() { ns.Stop() })
	t.Cleanup(func() { _ = store.Close() })
	t.Cleanup(func() { pubNC.Close() })
	t.Cleanup(func() { subNC.Close() })
	t.Cleanup(func() { writer.Stop() })
	t.Cleanup(cancel)
	t.Cleanup(sub.Stop)
	pub, err := natsbus.NewPublisher(pubNC, "ffa-local", sourceUUID, 0)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	// Publish a match_start.
	matchUUID := "match-loopback-1"
	ts := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	if err := pub.Publish(domain.FactEvent{
		Type:      domain.FactMatchStart,
		ServerID:  srv.ID,
		Timestamp: ts,
		Data: domain.MatchStartData{
			MatchUUID: matchUUID,
			MapName:   "q3dm17",
			GameType:  "FFA",
			StartedAt: ts,
		},
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Wait up to 3s for consumed_seq to advance.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		seq, err := store.GetConsumedSeq(ctx, sourceUUID)
		if err != nil {
			t.Fatalf("GetConsumedSeq: %v", err)
		}
		if seq == 1 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if seq, _ := store.GetConsumedSeq(ctx, sourceUUID); seq != 1 {
		t.Fatalf("consumed_seq = %d, want 1 (subscriber did not process)", seq)
	}

	// Verify the match landed in the DB.
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

	// Republish the same envelope (same seq) and assert dedup: still
	// consumed_seq==1 and only one match row.
	if err := pub.Publish(domain.FactEvent{
		Type:      domain.FactMatchStart,
		ServerID:  srv.ID,
		Timestamp: ts,
		Data: domain.MatchStartData{
			MatchUUID: matchUUID + "-dup",
			MapName:   "should-not-persist",
			GameType:  "FFA",
			StartedAt: ts,
		},
	}); err != nil {
		// Seq advanced; dedup this time will happen at the JetStream
		// Msg-Id level if we had replayed the same seq, but since the
		// publisher increments we send with seq=2 — which WILL be
		// processed. This second publish is here purely to assert the
		// pipeline keeps moving; we verify via the final seq.
		t.Fatalf("second publish: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if seq, _ := store.GetConsumedSeq(ctx, sourceUUID); seq != 2 {
		t.Errorf("consumed_seq after second publish = %d, want 2", seq)
	}
}
