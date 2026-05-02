package directory

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/storage"
)

type fakeGateStore struct {
	rows []storage.RemoteServer
	err  error
}

func (f *fakeGateStore) ListDirectoryGateEntries(ctx context.Context) ([]storage.RemoteServer, error) {
	return f.rows, f.err
}

func TestGateAllowIPLiteral(t *testing.T) {
	store := &fakeGateStore{
		rows: []storage.RemoteServer{
			{ID: 1, Source: "local", Key: "a", Address: "10.0.0.1:27960"},
			{ID: 2, Source: "local", Key: "b", Address: "[2001:db8::1]:27961"},
		},
	}
	g := newGate(store, time.Hour)
	g.refreshOnce(context.Background())

	if g.Size() != 2 {
		t.Fatalf("Size=%d, want 2", g.Size())
	}
	if id, ok := g.Allow(mustAddrPort(t, "10.0.0.1:27960")); !ok || id != 1 {
		t.Errorf("v4 lookup id=%d ok=%v", id, ok)
	}
	v6 := netip.AddrPortFrom(netip.MustParseAddr("2001:db8::1"), 27961)
	if id, ok := g.Allow(v6); !ok || id != 2 {
		t.Errorf("v6 lookup id=%d ok=%v", id, ok)
	}
	if _, ok := g.Allow(mustAddrPort(t, "10.0.0.1:99")); ok {
		t.Error("wrong port should not match")
	}
	if _, ok := g.Allow(mustAddrPort(t, "10.0.0.99:27960")); ok {
		t.Error("wrong ip should not match")
	}
}

func TestGateSkipsBadAddresses(t *testing.T) {
	store := &fakeGateStore{
		rows: []storage.RemoteServer{
			{ID: 1, Address: ""},
			{ID: 2, Address: "noport"},
			{ID: 3, Address: "10.0.0.1:notaport"},
			{ID: 4, Address: "10.0.0.4:27960"},
		},
	}
	g := newGate(store, time.Hour)
	g.refreshOnce(context.Background())
	if g.Size() != 1 {
		t.Errorf("expected only id=4 to land, got Size=%d", g.Size())
	}
	if _, ok := g.Allow(mustAddrPort(t, "10.0.0.4:27960")); !ok {
		t.Error("good row missing")
	}
}

func TestGateEmptyOnError(t *testing.T) {
	g := newGate(&fakeGateStore{err: context.Canceled}, time.Hour)
	g.refreshOnce(context.Background())
	if g.Size() != 0 {
		t.Errorf("Size=%d on store error, want 0", g.Size())
	}
}
