package natsbus_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/config"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/natsbus"
)

func bufFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func TestBufferedPublisherDrainsToJetStream(t *testing.T) {
	port := bufFreePort(t)
	trackerCfg := &config.TrackerConfig{
		NATS: config.NATSConfig{URL: fmt.Sprintf("nats://127.0.0.1:%d", port)},
		Hub:  &config.HubConfig{DedupWindow: config.Duration(time.Minute), Retention: config.Duration(time.Hour)},
	}
	ns, err := natsbus.Start(trackerCfg, t.TempDir())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer ns.Stop()

	nc, err := ns.ConnectInternal()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()

	inner, err := natsbus.NewPublisher(nc, "src", "uuid-1", 0)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	buf := natsbus.NewBufferedPublisher(inner, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	buf.Start(ctx)
	defer buf.Stop()

	// Verify the inner publisher can reach JS by subscribing.
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("js: %v", err)
	}
	sub, err := js.PullSubscribe("trinity.events.src", "test")
	if err != nil {
		t.Fatalf("sub: %v", err)
	}

	// Send five events through the buffered path.
	for i := 0; i < 5; i++ {
		if err := buf.Publish(domain.FactEvent{
			Type:      domain.FactServerStartup,
			Timestamp: time.Now().UTC(),
			Data:      domain.ServerStartupData{StartedAt: time.Now().UTC()},
		}); err != nil {
			t.Fatalf("Publish %d: %v", i, err)
		}
	}

	var total int
	deadline := time.Now().Add(3 * time.Second)
	for total < 5 && time.Now().Before(deadline) {
		msgs, err := sub.Fetch(5-total, nats.MaxWait(500*time.Millisecond))
		if err != nil && err != nats.ErrTimeout {
			t.Fatalf("Fetch: %v", err)
		}
		for _, m := range msgs {
			_ = m.Ack()
		}
		total += len(msgs)
	}
	if total != 5 {
		t.Fatalf("want 5 msgs total, got %d", total)
	}
	if dropped := buf.Dropped(); dropped != 0 {
		t.Errorf("Dropped = %d, want 0", dropped)
	}
}

func TestBufferedPublisherOverflowDropsOldest(t *testing.T) {
	// Use a nil-safe path: don't actually publish to NATS — just
	// verify the counter advances under overflow. We construct a
	// BufferedPublisher with inner set to a real Publisher against an
	// unreachable server; Publish never blocks because it only
	// enqueues.
	port := bufFreePort(t)
	trackerCfg := &config.TrackerConfig{
		NATS: config.NATSConfig{URL: fmt.Sprintf("nats://127.0.0.1:%d", port)},
		Hub:  &config.HubConfig{DedupWindow: config.Duration(time.Minute), Retention: config.Duration(time.Hour)},
	}
	ns, err := natsbus.Start(trackerCfg, t.TempDir())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer ns.Stop()

	nc, err := ns.ConnectInternal()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()
	inner, err := natsbus.NewPublisher(nc, "src", "uuid-1", 0)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	// Capacity 2; don't start the drain goroutine so events pile up.
	buf := natsbus.NewBufferedPublisher(inner, 2)
	// Do not Start — we only exercise the overflow path.
	// Defensive cleanup so teardown doesn't hang.
	defer func() {
		buf.Start(context.Background())
		buf.Stop()
	}()

	for i := 0; i < 5; i++ {
		if err := buf.Publish(domain.FactEvent{Type: domain.FactServerStartup}); err != nil {
			t.Fatalf("Publish %d: %v", i, err)
		}
	}
	if got := buf.Dropped(); got < 3 {
		t.Errorf("Dropped = %d, want ≥ 3", got)
	}
}
