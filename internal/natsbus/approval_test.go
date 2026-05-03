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

// TestProvisionedSourcePublishesStraightThrough exercises the pre-
// provisioning flow: admin creates the source first, hands creds to
// the remote operator, collector connects and publishes, events land
// directly in the DB with no intermediate pending/DLQ state.
func TestProvisionedSourcePublishesStraightThrough(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Hub-side infrastructure.
	port := approvalFreePort(t)
	cfg := &config.TrackerConfig{
		NATS: config.NATSConfig{URL: fmt.Sprintf("nats://127.0.0.1:%d", port)},
		Hub:  &config.HubConfig{DedupWindow: config.Duration(time.Minute), Retention: config.Duration(time.Hour)},
	}
	ns, err := natsbus.Start(cfg, t.TempDir(), nil)
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

	// --- Admin provisions the source up-front ---
	const source = "remote"
	if err := store.CreateUser(ctx, "owner", "x", false, nil); err != nil {
		t.Fatalf("seed owner user: %v", err)
	}
	owner, err := store.GetUserByUsername(ctx, "owner")
	if err != nil {
		t.Fatalf("lookup seeded owner: %v", err)
	}
	if err := store.CreateSource(ctx, source, true, &owner.ID); err != nil {
		t.Fatalf("CreateSource: %v", err)
	}
	writer.MarkSourceApproved(source)

	// Collector-side publisher + registrar.
	pubNC, err := ns.ConnectInternal()
	if err != nil {
		t.Fatalf("pub conn: %v", err)
	}
	reg := domain.Registration{
		Source:  source,
		Version: "1.12.0",
		Servers: []domain.RegdServer{{LocalID: 1, Key: "r1", Address: "r.example:27960"}},
	}
	registrar, err := natsbus.NewRegistrar(pubNC, reg.Source, reg.Version, "", func() []domain.RegdServer { return reg.Servers }, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("registrar: %v", err)
	}
	registrar.Start(ctx)

	pub, err := natsbus.NewPublisher(pubNC, reg.Source, 0)
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

	// Wait for the first registration to create the servers row.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if id, _ := store.ResolveServerIDForSource(ctx, source, reg.Servers[0].LocalID); id != 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if id, _ := store.ResolveServerIDForSource(ctx, source, reg.Servers[0].LocalID); id == 0 {
		t.Fatal("servers row never created from registration")
	}

	// Publish a match_start — should land in the DB immediately.
	matchUUID := "provisioned-match-1"
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

	deadline = time.Now().Add(2 * time.Second)
	var m *domain.Match
	for time.Now().Before(deadline) {
		m, _ = store.GetMatchByUUID(ctx, matchUUID)
		if m != nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if m == nil {
		t.Fatal("match not persisted")
	}
	if m.MapName != "q3dm17" {
		t.Errorf("MapName = %q", m.MapName)
	}
}
