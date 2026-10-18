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
	ns, err := natsbus.Start(trackerCfg, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer ns.Stop()

	nc, err := ns.ConnectInternal()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()

	inner, err := natsbus.NewPublisher(nc, "src", 0)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	buf := natsbus.NewBufferedPublisher(inner, 10, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	buf.Start(ctx)
	defer buf.Stop()

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("js: %v", err)
	}
	sub, err := js.PullSubscribe("trinity.events.src", "test")
	if err != nil {
		t.Fatalf("sub: %v", err)
	}

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
	port := bufFreePort(t)
	trackerCfg := &config.TrackerConfig{
		NATS: config.NATSConfig{URL: fmt.Sprintf("nats://127.0.0.1:%d", port)},
		Hub:  &config.HubConfig{DedupWindow: config.Duration(time.Minute), Retention: config.Duration(time.Hour)},
	}
	ns, err := natsbus.Start(trackerCfg, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer ns.Stop()

	nc, err := ns.ConnectInternal()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()
	inner, err := natsbus.NewPublisher(nc, "src", 0)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	// Capacity 2 with no drain goroutine, so events pile up.
	buf := natsbus.NewBufferedPublisher(inner, 2, nil)
	// Stop waits on the drain goroutine, so start it first.
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

func TestBufferedPublisherSpillsAndDrains(t *testing.T) {
	port := bufFreePort(t)
	trackerCfg := &config.TrackerConfig{
		NATS: config.NATSConfig{URL: fmt.Sprintf("nats://127.0.0.1:%d", port)},
		Hub:  &config.HubConfig{DedupWindow: config.Duration(time.Minute), Retention: config.Duration(time.Hour)},
	}
	ns, err := natsbus.Start(trackerCfg, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer ns.Stop()

	nc, err := ns.ConnectInternal()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()
	inner, err := natsbus.NewPublisher(nc, "src-spill", 0)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	dataDir := t.TempDir()
	spill, err := natsbus.NewSpillQueue(dataDir)
	if err != nil {
		t.Fatalf("NewSpillQueue: %v", err)
	}
	defer spill.Close()

	// Capacity 4, HWM at 80% = 3. With 20 published events fast, most
	// will land in spill before the drain catches up.
	buf := natsbus.NewBufferedPublisher(inner, 4, spill)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	buf.Start(ctx)
	defer buf.Stop()

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("js: %v", err)
	}
	sub, err := js.PullSubscribe("trinity.events.src-spill", "test-spill")
	if err != nil {
		t.Fatalf("sub: %v", err)
	}

	const n = 20
	for i := 0; i < n; i++ {
		if err := buf.Publish(domain.FactEvent{
			Type:      domain.FactServerStartup,
			Timestamp: time.Now().UTC(),
			Data:      domain.ServerStartupData{StartedAt: time.Now().UTC()},
		}); err != nil {
			t.Fatalf("Publish %d: %v", i, err)
		}
	}

	var total int
	deadline := time.Now().Add(5 * time.Second)
	for total < n && time.Now().Before(deadline) {
		msgs, err := sub.Fetch(n-total, nats.MaxWait(500*time.Millisecond))
		if err != nil && err != nats.ErrTimeout {
			t.Fatalf("Fetch: %v", err)
		}
		for _, m := range msgs {
			_ = m.Ack()
		}
		total += len(msgs)
	}
	if total != n {
		t.Fatalf("got %d/%d events through", total, n)
	}
	if dropped := buf.Dropped(); dropped != 0 {
		t.Errorf("Dropped = %d, want 0 (nothing should be lost)", dropped)
	}
}
