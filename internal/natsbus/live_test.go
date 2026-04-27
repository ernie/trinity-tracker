package natsbus_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/config"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/hub"
	"github.com/ernie/trinity-tracker/internal/natsbus"
	"github.com/ernie/trinity-tracker/internal/storage"
)

// recordingSink collects events broadcast to it for inspection.
type recordingSink struct {
	mu     sync.Mutex
	events []domain.Event
}

func (s *recordingSink) Broadcast(e domain.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
}

func (s *recordingSink) snapshot() []domain.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.Event, len(s.events))
	copy(out, s.events)
	return out
}

func TestLiveEventRoundTrip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// cancel is registered via t.Cleanup further down so it runs
	// BEFORE writer.Stop (which waits on goroutines that exit on
	// ctx.Done).
	_ = cancel

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
	t.Cleanup(ns.Stop)

	// Hub-side store + writer so EnrichEvent is available and the
	// resolver can map (source_uuid, rsid) → servers.id.
	store, err := storage.New(filepath.Join(t.TempDir(), "trinity.db"))
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	srv := &domain.Server{Name: "ffa", Address: "127.0.0.1:27960"}
	if err := store.UpsertServer(ctx, srv); err != nil {
		t.Fatalf("UpsertServer: %v", err)
	}
	const sourceUUID = "remote-source-uuid"
	if err := store.TagLocalServerSource(ctx, srv.ID, sourceUUID, srv.ID); err != nil {
		t.Fatalf("TagLocalServerSource: %v", err)
	}

	writer := hub.NewWriter(store)
	writer.Start(ctx)
	// Order matters: writer.Stop is registered first so it runs AFTER
	// cancel (cleanups run LIFO); cancel frees the linkCodeCleanupLoop
	// goroutine that writer.Stop would otherwise wait on forever.
	t.Cleanup(writer.Stop)
	t.Cleanup(cancel)

	// Subscriber on the hub side.
	subNC, err := ns.ConnectInternal(nats.Name("live-subscriber"))
	if err != nil {
		t.Fatalf("hub connect: %v", err)
	}
	t.Cleanup(subNC.Close)

	sink := &recordingSink{}
	ls, err := natsbus.NewLiveSubscriber(subNC, store, writer, sink, "")
	if err != nil {
		t.Fatalf("NewLiveSubscriber: %v", err)
	}
	t.Cleanup(ls.Stop)

	// Publisher on the (remote) collector side.
	pubNC, err := ns.ConnectInternal(nats.Name("live-publisher"))
	if err != nil {
		t.Fatalf("collector connect: %v", err)
	}
	t.Cleanup(pubNC.Close)
	lp, err := natsbus.NewLivePublisher(pubNC, "remote-source", sourceUUID)
	if err != nil {
		t.Fatalf("NewLivePublisher: %v", err)
	}

	// Publish a frag with RemoteServerID = local_id = servers.id so
	// ResolveServerIDForSource returns the same id.
	frag := domain.Event{
		Type:      domain.EventFrag,
		ServerID:  srv.ID,
		Timestamp: time.Now().UTC(),
		Data: domain.FragEvent{
			Fragger:     "Alice",
			Victim:      "Bob",
			Weapon:      "rocket",
			FraggerGUID: "guid-alice",
			VictimGUID:  "guid-bob",
		},
	}
	if err := lp.PublishLive(frag); err != nil {
		t.Fatalf("PublishLive: %v", err)
	}

	// Wait for the subscriber goroutine to receive and broadcast.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(sink.snapshot()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	events := sink.snapshot()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(events), events)
	}
	got := events[0]
	if got.Type != domain.EventFrag {
		t.Errorf("Type = %q, want %q", got.Type, domain.EventFrag)
	}
	if got.ServerID != srv.ID {
		t.Errorf("ServerID = %d, want %d", got.ServerID, srv.ID)
	}
	fragData, ok := got.Data.(domain.FragEvent)
	if !ok {
		t.Fatalf("Data type = %T, want domain.FragEvent", got.Data)
	}
	if fragData.Fragger != "Alice" || fragData.Victim != "Bob" || fragData.Weapon != "rocket" {
		t.Errorf("frag data mismatch: %+v", fragData)
	}
}

func TestLiveSubscriberSkipsSelfSource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// cancel is registered via t.Cleanup further down so it runs
	// BEFORE writer.Stop (which waits on goroutines that exit on
	// ctx.Done).
	_ = cancel

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
	t.Cleanup(ns.Stop)

	store, err := storage.New(filepath.Join(t.TempDir(), "trinity.db"))
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	writer := hub.NewWriter(store)
	writer.Start(ctx)
	// Order matters: writer.Stop is registered first so it runs AFTER
	// cancel (cleanups run LIFO); cancel frees the linkCodeCleanupLoop
	// goroutine that writer.Stop would otherwise wait on forever.
	t.Cleanup(writer.Stop)
	t.Cleanup(cancel)

	subNC, err := ns.ConnectInternal()
	if err != nil {
		t.Fatalf("sub connect: %v", err)
	}
	t.Cleanup(subNC.Close)

	const selfUUID = "self-uuid"
	sink := &recordingSink{}
	ls, err := natsbus.NewLiveSubscriber(subNC, store, writer, sink, selfUUID)
	if err != nil {
		t.Fatalf("NewLiveSubscriber: %v", err)
	}
	t.Cleanup(ls.Stop)

	pubNC, err := ns.ConnectInternal()
	if err != nil {
		t.Fatalf("pub connect: %v", err)
	}
	t.Cleanup(pubNC.Close)
	lp, err := natsbus.NewLivePublisher(pubNC, "local-source", selfUUID)
	if err != nil {
		t.Fatalf("NewLivePublisher: %v", err)
	}

	if err := lp.PublishLive(domain.Event{
		Type:      domain.EventFrag,
		Timestamp: time.Now().UTC(),
		Data:      domain.FragEvent{Fragger: "x", Victim: "y"},
	}); err != nil {
		t.Fatalf("PublishLive: %v", err)
	}
	// Give the subscriber a moment to (not) process the message.
	time.Sleep(150 * time.Millisecond)
	if got := sink.snapshot(); len(got) != 0 {
		t.Errorf("expected 0 events (self-loop skipped), got %d: %+v", len(got), got)
	}
}
