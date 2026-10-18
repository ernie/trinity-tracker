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

type fakeGateConns map[string][]netip.Addr

func (f fakeGateConns) ConnectedClientIPs(userPubKey string) []netip.Addr {
	return f[userPubKey]
}

func TestGateLocalAdmitsLoopback(t *testing.T) {
	store := &fakeGateStore{
		rows: []storage.RemoteServer{
			{ID: 1, Source: "hub", Address: "trinity.run:27960", IsRemote: false},
		},
	}
	g := newGate(store, fakeGateConns{}, time.Hour)
	g.refreshOnce(context.Background())

	if id, ok := g.Allow(mustAddrPort(t, "127.0.0.1:27960")); !ok || id != 1 {
		t.Errorf("loopback v4 lookup id=%d ok=%v", id, ok)
	}
	v6 := netip.AddrPortFrom(netip.MustParseAddr("::1"), 27960)
	if id, ok := g.Allow(v6); !ok || id != 1 {
		t.Errorf("loopback v6 lookup id=%d ok=%v", id, ok)
	}
	// Wrong port: nothing matches.
	if _, ok := g.Allow(mustAddrPort(t, "127.0.0.1:99")); ok {
		t.Error("wrong port should not match")
	}
	// Off-host IP: should not match.
	if _, ok := g.Allow(mustAddrPort(t, "203.0.113.99:27960")); ok {
		t.Error("non-local ip should not match")
	}
}

// The port comes from the address column; its hostname is ignored.
func TestGateRemoteUsesConnIP(t *testing.T) {
	const piPub = "UABCDEF1234567890PIPUBKEY"
	store := &fakeGateStore{
		rows: []storage.RemoteServer{
			{ID: 7, Source: "pi-3", Address: "pi.trinity.run:27960", IsRemote: true, UserPubKey: piPub},
		},
	}
	conns := fakeGateConns{
		piPub: {netip.MustParseAddr("203.0.113.42")},
	}
	g := newGate(store, conns, time.Hour)
	g.refreshOnce(context.Background())

	if id, ok := g.Allow(mustAddrPort(t, "203.0.113.42:27960")); !ok || id != 7 {
		t.Errorf("conn IP lookup id=%d ok=%v", id, ok)
	}
	// The hostname's DNS answer isn't admitted.
	if _, ok := g.Allow(mustAddrPort(t, "198.51.100.10:27960")); ok {
		t.Error("only the live connection IP should be admitted")
	}
}

func TestGateRemoteWithoutConnDropped(t *testing.T) {
	const piPub = "UABCDEF1234567890PIPUBKEY"
	store := &fakeGateStore{
		rows: []storage.RemoteServer{
			{ID: 7, Source: "pi-3", Address: "pi.trinity.run:27960", IsRemote: true, UserPubKey: piPub},
		},
	}
	g := newGate(store, fakeGateConns{}, time.Hour)
	g.refreshOnce(context.Background())
	if g.Size() != 0 {
		t.Errorf("Size=%d, want 0 when no collector connected", g.Size())
	}
}

func TestGateRemoteWithoutPubkeySkipped(t *testing.T) {
	store := &fakeGateStore{
		rows: []storage.RemoteServer{
			{ID: 7, Source: "pi-3", Address: "pi.trinity.run:27960", IsRemote: true},
		},
	}
	g := newGate(store, fakeGateConns{}, time.Hour)
	g.refreshOnce(context.Background())
	if g.Size() != 0 {
		t.Errorf("Size=%d, want 0 when source has no minted creds", g.Size())
	}
}

// During a reconnect a source briefly has connections from its old and
// new IPs, and both are admitted.
func TestGateRemoteMultiConn(t *testing.T) {
	const piPub = "UABCDEF1234567890PIPUBKEY"
	store := &fakeGateStore{
		rows: []storage.RemoteServer{
			{ID: 7, Source: "pi-3", Address: "pi.trinity.run:27960", IsRemote: true, UserPubKey: piPub},
		},
	}
	conns := fakeGateConns{
		piPub: {
			netip.MustParseAddr("203.0.113.42"),
			netip.MustParseAddr("198.51.100.7"),
		},
	}
	g := newGate(store, conns, time.Hour)
	g.refreshOnce(context.Background())

	if id, ok := g.Allow(mustAddrPort(t, "203.0.113.42:27960")); !ok || id != 7 {
		t.Errorf("old-IP lookup id=%d ok=%v", id, ok)
	}
	if id, ok := g.Allow(mustAddrPort(t, "198.51.100.7:27960")); !ok || id != 7 {
		t.Errorf("new-IP lookup id=%d ok=%v", id, ok)
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
	g := newGate(store, fakeGateConns{}, time.Hour)
	g.refreshOnce(context.Background())
	// id=4 (local) lands at every local interface; check via loopback.
	if id, ok := g.Allow(mustAddrPort(t, "127.0.0.1:27960")); !ok || id != 4 {
		t.Errorf("good row missing: id=%d ok=%v", id, ok)
	}
	// Bad-address rows must not contribute any entry.
	if _, ok := g.Allow(mustAddrPort(t, "127.0.0.1:0")); ok {
		t.Error("empty-address row leaked into gate")
	}
}

func TestGateEmptyOnError(t *testing.T) {
	g := newGate(&fakeGateStore{err: context.Canceled}, fakeGateConns{}, time.Hour)
	g.refreshOnce(context.Background())
	if g.Size() != 0 {
		t.Errorf("Size=%d on store error, want 0", g.Size())
	}
}
