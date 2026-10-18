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

// gateStore is the part of *storage.Store the gate reads; tests supply
// a fake.
type gateStore interface {
	ListDirectoryGateEntries(ctx context.Context) ([]storage.RemoteServer, error)
}

// gateConns reports the IPs of the authenticated NATS connections for a
// user pubkey: usually one, briefly two during a reconnect, none when
// the collector is offline. *natsbus.Server implements it.
type gateConns interface {
	ConnectedClientIPs(userPubKey string) []netip.Addr
}

// gate admits only heartbeats whose UDP source (ip, port) matches a
// row in servers. The map is rebuilt on a ticker, so a collector that
// reconnects from a new IP is picked up without a hub restart.
//
// A remote row takes its IP from the source's live NATS connection, as
// the kernel recorded it.
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

// Allow reports whether srcAddr is in the last loaded map, with the
// matching servers.id for logging.
func (g *gate) Allow(srcAddr netip.AddrPort) (int64, bool) {
	m := g.current.Load()
	if m == nil {
		return 0, false
	}
	id, ok := (*m)[srcAddr]
	return id, ok
}

// Run refreshes the gate every g.refresh until ctx is canceled. The
// first refresh runs before Run starts waiting, so the gate is loaded
// before the first heartbeat.
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

// refreshOnce rebuilds the map from the database. A remote row whose
// collector is offline is left out until a refresh after it reconnects.
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
				// No credentials yet, so no connection to match.
				continue
			}
			for _, ip := range g.conns.ConnectedClientIPs(r.UserPubKey) {
				next[netip.AddrPortFrom(ip.Unmap(), port)] = r.ID
			}
			continue
		}

		// A local row admits the row's port from any of this host's own
		// addresses, loopback included. r.Address's host is the public
		// hostname for URLs and plays no part here.
		_ = host
		for _, ip := range localAddrs {
			next[netip.AddrPortFrom(ip.Unmap(), port)] = r.ID
		}
	}
	g.current.Store(&next)
}

// hostLocalAddrs returns the addresses local rows accept heartbeats
// from: loopback plus the primary outbound IPv4 and IPv6 address.
//
// A connected UDP socket finds the source IP the kernel would route
// from, sending nothing. It needs no AF_NETLINK, which systemd sandboxes
// often block with RestrictAddressFamilies.
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
	// Any routable target works; well-known anycast addresses are the
	// likeliest to have a default route.
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

// Size returns how many addresses the gate admits.
func (g *gate) Size() int {
	m := g.current.Load()
	if m == nil {
		return 0
	}
	return len(*m)
}
