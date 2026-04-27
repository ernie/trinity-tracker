package natsbus_test

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/config"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/hub"
	"github.com/ernie/trinity-tracker/internal/natsbus"
	"github.com/ernie/trinity-tracker/internal/storage"
)

func approvalFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// TestPendingSourceApprovalDrainsDLQ walks through the full phase-6b
// flow: a remote collector starts publishing before approval, events
// land in the DLQ, admin approves, DLQ drains into the DB.
func TestPendingSourceApprovalDrainsDLQ(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Hub-side infrastructure.
	port := approvalFreePort(t)
	cfg := &config.TrackerConfig{
		NATS: config.NATSConfig{URL: fmt.Sprintf("nats://127.0.0.1:%d", port)},
		Hub:  &config.HubConfig{DedupWindow: config.Duration(time.Minute), Retention: config.Duration(time.Hour)},
	}
	ns, err := natsbus.Start(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	store, err := storage.New(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatalf("storage: %v", err)
	}

	writer := hub.NewWriter(store)
	writer.Start(ctx)

	subNC, err := ns.ConnectInternal()
	if err != nil {
		t.Fatalf("sub conn: %v", err)
	}
	sub, err := natsbus.NewSubscriber(subNC, writer)
	if err != nil {
		t.Fatalf("subscriber: %v", err)
	}
	sub.Start(ctx)

	regSub, err := natsbus.NewRegistrationSubscriber(subNC, writer)
	if err != nil {
		t.Fatalf("reg sub: %v", err)
	}

	// Collector-side publisher + registrar.
	pubNC, err := ns.ConnectInternal()
	if err != nil {
		t.Fatalf("pub conn: %v", err)
	}
	const sourceUUID = "remote-source-uuid"
	reg := domain.Registration{
		Source:     "remote",
		SourceUUID: sourceUUID,
		Version:    "1.12.0",
		Servers:    []domain.RegdServer{{LocalID: 1, Name: "r1", Address: "r.example:27960"}},
	}
	registrar, err := natsbus.NewRegistrar(pubNC, reg.Source, sourceUUID, reg.Version, func() []domain.RegdServer { return reg.Servers }, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("registrar: %v", err)
	}
	registrar.Start(ctx)

	pub, err := natsbus.NewPublisher(pubNC, reg.Source, sourceUUID, 0)
	if err != nil {
		t.Fatalf("publisher: %v", err)
	}

	// Teardown in dependency order.
	t.Cleanup(func() { ns.Stop() })
	t.Cleanup(func() { _ = store.Close() })
	t.Cleanup(func() { pubNC.Close() })
	t.Cleanup(func() { subNC.Close() })
	t.Cleanup(func() { writer.Stop() })
	t.Cleanup(cancel)
	t.Cleanup(regSub.Stop)
	t.Cleanup(registrar.Stop)
	t.Cleanup(sub.Stop)

	// Wait for the hub to record the source as pending.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rows, _ := store.ListPendingSources(ctx)
		if len(rows) == 1 && rows[0].SourceUUID == sourceUUID {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if rows, _ := store.ListPendingSources(ctx); len(rows) != 1 {
		t.Fatalf("pending sources = %d, want 1", len(rows))
	}

	// Publish a match_start while still pending — should DLQ.
	matchUUID := "pending-match-1"
	if err := pub.Publish(domain.FactEvent{
		Type:      domain.FactMatchStart,
		ServerID:  reg.Servers[0].LocalID,
		Timestamp: time.Now().UTC(),
		Data: domain.MatchStartData{
			MatchUUID:         matchUUID,
			MapName:           "q3dm17",
			GameType:          "FFA",
			StartedAt:         time.Now().UTC(),
			HandshakeRequired: true,
		},
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// DB should NOT yet have the match row.
	if m, _ := store.GetMatchByUUID(ctx, matchUUID); m != nil {
		t.Errorf("match persisted while pending: %+v", m)
	}

	// Admin approves. DLQ drains.
	if err := writer.ApproveSource(ctx, reg, "https://remote.example"); err != nil {
		t.Fatalf("approve: %v", err)
	}

	// Give the drain a beat to land.
	time.Sleep(100 * time.Millisecond)

	m, err := store.GetMatchByUUID(ctx, matchUUID)
	if err != nil {
		t.Fatalf("lookup match: %v", err)
	}
	if m == nil {
		t.Fatal("match still not persisted after approval")
	}
	if m.MapName != "q3dm17" {
		t.Errorf("MapName = %q", m.MapName)
	}
}
