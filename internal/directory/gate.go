package directory

import (
	"context"
	"log"
	"net"
	"net/netip"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/ernie/trinity-tracker/internal/storage"
)

// gateStore is the read-only slice of *storage.Store the gate needs.
// Defined as an interface so tests can supply a fake without depending
// on SQLite.
type gateStore interface {
	ListDirectoryGateEntries(ctx context.Context) ([]storage.RemoteServer, error)
}

// gate is the membership filter: only heartbeats whose source addr
// matches a row in `servers` (after DNS resolution if the row stores a
// hostname) are admitted. The map is rebuilt on a ticker so operator
// changes propagate without a hub restart.
type gate struct {
	store    gateStore
	resolver *net.Resolver
	refresh  time.Duration

	current atomic.Pointer[map[netip.AddrPort]int64]
}

func newGate(store gateStore, refresh time.Duration) *gate {
	g := &gate{
		store:    store,
		resolver: net.DefaultResolver,
		refresh:  refresh,
	}
	empty := make(map[netip.AddrPort]int64)
	g.current.Store(&empty)
	return g
}

// Allow checks whether srcAddr is one of the resolved (ip, port)
// tuples we last loaded from the servers table. Returns the matching
// servers.id (for logging) and a boolean.
func (g *gate) Allow(srcAddr netip.AddrPort) (int64, bool) {
	m := g.current.Load()
	if m == nil {
		return 0, false
	}
	id, ok := (*m)[srcAddr]
	return id, ok
}

// Run blocks until ctx is cancelled, refreshing the gate every
// g.refresh. It also performs an initial refresh synchronously so the
// gate is populated before the first heartbeat hits it.
func (g *gate) Run(ctx context.Context) {
	g.refreshOnce(ctx)
	t := time.NewTicker(g.refresh)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			g.refreshOnce(ctx)
		}
	}
}

// refreshOnce queries the DB and replaces the gate's resolved map.
// DNS failures on individual rows are logged at debug; the previous
// resolution for that row is dropped (we re-resolve from scratch each
// pass — keeps the code simple and avoids stale-resolution drift).
func (g *gate) refreshOnce(ctx context.Context) {
	rows, err := g.store.ListDirectoryGateEntries(ctx)
	if err != nil {
		log.Printf("directory.gate: list servers: %v", err)
		return
	}
	next := make(map[netip.AddrPort]int64, len(rows))
	for _, r := range rows {
		host, portStr, err := net.SplitHostPort(r.Address)
		if err != nil {
			log.Printf("directory.gate: server id=%d address=%q: %v", r.ID, r.Address, err)
			continue
		}
		port64, err := strconv.ParseUint(portStr, 10, 16)
		if err != nil {
			log.Printf("directory.gate: server id=%d port=%q: %v", r.ID, portStr, err)
			continue
		}
		port := uint16(port64)

		if ip, err := netip.ParseAddr(host); err == nil {
			next[netip.AddrPortFrom(ip.Unmap(), port)] = r.ID
			continue
		}

		// Hostname — resolve. Use a per-row timeout so one slow lookup
		// can't stall the whole refresh.
		lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		ips, err := g.resolver.LookupNetIP(lookupCtx, "ip", host)
		cancel()
		if err != nil {
			log.Printf("directory.gate: resolve %s (server id=%d): %v", host, r.ID, err)
			continue
		}
		for _, ip := range ips {
			next[netip.AddrPortFrom(ip.Unmap(), port)] = r.ID
		}
	}
	g.current.Store(&next)
}

// Size returns the current gate cardinality. For metrics/tests.
func (g *gate) Size() int {
	m := g.current.Load()
	if m == nil {
		return 0
	}
	return len(*m)
}
