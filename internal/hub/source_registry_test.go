package hub

import (
	"context"
	"testing"
	"time"
)

func TestSourceRegistryStateApprovedFromDB(t *testing.T) {
	_, store := newTestWriter(t)
	ctx := context.Background()

	if err := store.CreateSource(ctx, "remote-1", true); err != nil {
		t.Fatalf("create: %v", err)
	}

	reg := NewSourceRegistry(store)
	state, err := reg.State(ctx, "remote-1")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if state != SourceApproved {
		t.Errorf("state = %v, want SourceApproved", state)
	}
}

func TestSourceRegistryStateUnknownForMissing(t *testing.T) {
	_, store := newTestWriter(t)
	reg := NewSourceRegistry(store)
	state, err := reg.State(context.Background(), "uuid-unknown")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if state != SourceUnknown {
		t.Errorf("state = %v, want SourceUnknown", state)
	}
}

func TestSourceRegistryMarkApprovedOverridesBlock(t *testing.T) {
	_, store := newTestWriter(t)
	reg := NewSourceRegistry(store)
	reg.MarkBlocked("u")
	if state, _ := reg.State(context.Background(), "u"); state != SourceBlocked {
		t.Fatalf("pre-check state = %v", state)
	}
	reg.MarkApproved("u")
	if state, _ := reg.State(context.Background(), "u"); state != SourceApproved {
		t.Errorf("post-approve state = %v", state)
	}
}

func TestSourceRegistryUnknownNegativeCache(t *testing.T) {
	_, store := newTestWriter(t)
	reg := NewSourceRegistry(store)

	base := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	reg.now = func() time.Time { return base }

	if state, _ := reg.State(context.Background(), "uuid-x"); state != SourceUnknown {
		t.Fatalf("first state = %v", state)
	}
	reg.mu.Lock()
	if _, ok := reg.unknownSince["uuid-x"]; !ok {
		reg.mu.Unlock()
		t.Fatal("expected unknownSince entry after first miss")
	}
	reg.mu.Unlock()

	// Approving via the DB inside the TTL should still return Unknown
	// until the cache entry ages out.
	if err := store.CreateSource(context.Background(), "uuid-x", true); err != nil {
		t.Fatalf("create: %v", err)
	}

	if state, _ := reg.State(context.Background(), "uuid-x"); state != SourceUnknown {
		t.Errorf("within TTL state = %v, want SourceUnknown (cache hit)", state)
	}

	reg.now = func() time.Time { return base.Add(UnknownCheckTTL + time.Second) }
	if state, _ := reg.State(context.Background(), "uuid-x"); state != SourceApproved {
		t.Errorf("post-TTL state = %v, want SourceApproved", state)
	}
}
