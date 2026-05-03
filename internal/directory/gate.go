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

// gateConns reports the IPs of every currently-authenticated NATS
// connection for a given user pubkey (zero, one, or — briefly during a
// reconnect overlap — more than one). Empty slice means "no live
// connection"; the gate will exclude that row.
//
// Implemented by *natsbus.Server in production; fake in tests.
type gateConns interface {
	ConnectedClientIPs(userPubKey string) []netip.Addr
}

// gate is the membership filter: only heartbeats whose UDP source
// (ip, port) matches a row in `servers` are admitted. The map is
// rebuilt on a ticker so collector reconnects (and any IP changes that
// come with them) propagate without a hub restart.
//
// Remote rows (is_remote=1) get their IP from the live NATS connection
// authenticated for that source — kernel-recorded, so it updates the
// moment a collector reconnects from a new public IP.
type gate struct {
	store   gateStore
	conns   gateConns
	refresh time.Duration

	current atomic.Pointer[map[netip.AddrPort]int64]
}

func newGate(store gateStore, conns gateConns, refresh time.Duration) *gate {
	g := &gate{
		store:   store,
		conns:   conns,
		refresh: refresh,
	}
	empty := make(map[netip.AddrPort]int64)
	g.current.Store(&empty)
	return g
}

// Allow checks whether srcAddr is one of the (ip, port) tuples we last
// loaded. Returns the matching servers.id (for logging) and a boolean.
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
// Remote rows whose collectors are not currently connected are silently
// dropped; they'll reappear on the next refresh after the collector
// reconnects.
func (g *gate) refreshOnce(ctx context.Context) {
	rows, err := g.store.ListDirectoryGateEntries(ctx)
	if err != nil {
		log.Printf("directory.gate: list servers: %v", err)
		return
	}
	next := make(map[netip.AddrPort]int64, len(rows))
	localAddrs := hostLocalAddrs()
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

		if r.IsRemote {
			if r.UserPubKey == "" {
				// Source approved but no creds minted yet — no
				// authenticated connection to identify, so the row
				// can't be admitted. Picked up automatically on the
				// next refresh after MintUserCreds.
				continue
			}
			for _, ip := range g.conns.ConnectedClientIPs(r.UserPubKey) {
				next[netip.AddrPortFrom(ip.Unmap(), port)] = r.ID
			}
			continue
		}

		// Local row: trusted by virtue of locality. Admit heartbeats
		// from any of this host's own interfaces (including loopback)
		// for the row's port. r.Address's host portion is informational
		// — it's the user-facing hostname for demo download URLs etc.,
		// not an authentication signal here.
		_ = host
		for _, ip := range localAddrs {
			next[netip.AddrPortFrom(ip.Unmap(), port)] = r.ID
		}
	}
	g.current.Store(&next)
}

// hostLocalAddrs returns the IPs this host accepts heartbeats from for
// local rows: loopback (always) plus the primary outbound IP for v4
// and v6 (if available) discovered via the kernel's routing table.
//
// Uses a connected UDP socket as a netlink-free probe: net.Dial picks
// the source IP the kernel would use for traffic to the target, and
// no packets actually leave the box (UDP "connect" only). This avoids
// AF_NETLINK, which is commonly blocked by systemd sandboxes via
// RestrictAddressFamilies.
func hostLocalAddrs() []netip.Addr {
	out := []netip.Addr{
		netip.MustParseAddr("127.0.0.1"),
		netip.MustParseAddr("::1"),
	}
	seen := map[netip.Addr]struct{}{out[0]: {}, out[1]: {}}
	add := func(addr netip.Addr) {
		addr = addr.Unmap()
		if _, dup := seen[addr]; dup {
			return
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	// Targets are arbitrary; the kernel only consults the routing
	// table, no traffic is sent until Write. Pick well-known anycast
	// addresses to maximise the chance a default route exists.
	for _, probe := range []struct{ network, target string }{
		{"udp4", "8.8.8.8:80"},
		{"udp6", "[2001:4860:4860::8888]:80"},
	} {
		conn, err := net.Dial(probe.network, probe.target)
		if err != nil {
			continue
		}
		la, ok := conn.LocalAddr().(*net.UDPAddr)
		conn.Close()
		if !ok {
			continue
		}
		if addr, ok := netip.AddrFromSlice(la.IP); ok {
			add(addr)
		}
	}
	return out
}

// Size returns the current gate cardinality. For metrics/tests.
func (g *gate) Size() int {
	m := g.current.Load()
	if m == nil {
		return 0
	}
	return len(*m)
}
