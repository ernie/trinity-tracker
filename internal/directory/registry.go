package directory

import (
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ernie/trinity-tracker/internal/storage"
)

// regEntry is one validated server in the directory's in-memory list.
// expiresAt advances every time we see a fresh infoResponse for this
// addr; the sweep removes entries whose deadline has passed.
type regEntry struct {
	addr        netip.AddrPort
	protocol    int
	gamename    string
	engine      string // exact `engine` infostring value (e.g. "trinity-engine/0.4.2")
	clients     int
	maxClients  int
	gametype    int
	validatedAt time.Time
	expiresAt   time.Time
	serverID    int64 // hub's servers.id, kept for logging
}

// registry holds every validated server. Reads (getservers) snapshot
// under RLock; writes (Upsert/sweep) take a Lock. The map size is
// bounded by maxServers — beyond that, new addrs are refused but
// existing ones still refresh.
type registry struct {
	mu         sync.RWMutex
	now        func() time.Time
	expiry     time.Duration
	maxServers int
	entries    map[netip.AddrPort]*regEntry
}

func newRegistry(expiry time.Duration, maxServers int, now func() time.Time) *registry {
	if now == nil {
		now = time.Now
	}
	return &registry{
		now:        now,
		expiry:     expiry,
		maxServers: maxServers,
		entries:    make(map[netip.AddrPort]*regEntry),
	}
}

// Upsert records or refreshes a validated entry. Returns false if the
// registry is at capacity AND the addr is new (existing entries always
// refresh, even if we're at the cap).
func (r *registry) Upsert(addr netip.AddrPort, info map[string]string, serverID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.entries[addr]
	if !ok && len(r.entries) >= r.maxServers {
		return false
	}

	now := r.now()
	entry := existing
	if entry == nil {
		entry = &regEntry{addr: addr, serverID: serverID}
		r.entries[addr] = entry
	}
	entry.protocol, _ = strconv.Atoi(info["protocol"])
	entry.gamename = info["gamename"]
	entry.engine = info["engine"]
	entry.clients, _ = strconv.Atoi(info["clients"])
	entry.maxClients, _ = strconv.Atoi(info["sv_maxclients"])
	entry.gametype, _ = strconv.Atoi(info["gametype"])
	entry.validatedAt = now
	entry.expiresAt = now.Add(r.expiry)
	return true
}

// Snapshot returns a copy of the live registry suitable for filtering
// outside the lock.
func (r *registry) Snapshot() []regEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]regEntry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, *e)
	}
	return out
}

// Sweep drops expired entries. Returns the number removed.
func (r *registry) Sweep() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	removed := 0
	for k, v := range r.entries {
		if now.After(v.expiresAt) {
			delete(r.entries, k)
			removed++
		}
	}
	return removed
}

// Len returns the current registry size. For metrics/tests.
func (r *registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}

// Load replaces the registry's contents with the supplied entries.
// Used at startup to seed from a persisted snapshot. The caller is
// responsible for any freshness or membership filtering — see
// decideRestore.
func (r *registry) Load(entries []regEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = make(map[netip.AddrPort]*regEntry, len(entries))
	for i := range entries {
		e := entries[i]
		r.entries[e.addr] = &e
	}
}

// decideRestore filters a persisted snapshot for restoration.
//
// Top-level freshness gate: if the most recent validated_at across
// rows is older than freshness, the whole snapshot is rejected
// (clearAll=true) — the hub was down longer than we'd trust as a
// routine restart, so we'd rather show an empty list than stale data.
//
// Per-row filter: rows whose own expires_at has already passed are
// dropped.
//
// We intentionally do NOT gate-filter restored entries by current
// membership. Remote collectors take seconds to detect a NATS
// disconnect and reconnect after a hub restart, so the gate is
// briefly empty for remote sources at restore time — filtering here
// would silently drop every remote registration on every restart.
// Worst case for skipping the check: an admin deprovisioned a
// source within the freshness window, so we briefly advertise an
// addr whose underlying row is gone. The entry will expire on its
// own at the original expires_at (or sooner via sweep), exactly
// matching the in-memory behavior during normal operation.
func decideRestore(
	rows []storage.DirectoryRegistration,
	freshness time.Duration,
	now time.Time,
) (keep []regEntry, clearAll bool) {
	if len(rows) == 0 {
		return nil, false
	}
	var newest time.Time
	for _, r := range rows {
		if r.ValidatedAt.After(newest) {
			newest = r.ValidatedAt
		}
	}
	if now.Sub(newest) > freshness {
		return nil, true
	}
	keep = make([]regEntry, 0, len(rows))
	for _, r := range rows {
		if !r.ExpiresAt.After(now) {
			continue
		}
		addr, err := netip.ParseAddrPort(r.Addr)
		if err != nil {
			continue
		}
		keep = append(keep, regEntry{
			addr:        addr,
			protocol:    r.Protocol,
			gamename:    r.Gamename,
			engine:      r.Engine,
			clients:     r.Clients,
			maxClients:  r.MaxClients,
			gametype:    r.Gametype,
			validatedAt: r.ValidatedAt,
			expiresAt:   r.ExpiresAt,
			serverID:    r.ServerID,
		})
	}
	return keep, false
}

// toPersisted converts the registry's current entries to the form
// the storage layer accepts. Called once at graceful shutdown.
func (r *registry) toPersisted() []storage.DirectoryRegistration {
	snap := r.Snapshot()
	if len(snap) == 0 {
		return nil
	}
	out := make([]storage.DirectoryRegistration, 0, len(snap))
	for _, e := range snap {
		out = append(out, storage.DirectoryRegistration{
			Addr:        e.addr.String(),
			ServerID:    e.serverID,
			Protocol:    e.protocol,
			Gamename:    e.gamename,
			Engine:      e.engine,
			Clients:     e.clients,
			MaxClients:  e.maxClients,
			Gametype:    e.gametype,
			ValidatedAt: e.validatedAt,
			ExpiresAt:   e.expiresAt,
		})
	}
	return out
}

// matchGamename compares an entry's gamename to the requested filter.
// Q3 servers without an `fs_game` cvar set advertise no gamename in
// infoResponse, in which case dpmaster treats it as the default
// gamename "Quake3Arena". We do the same so vanilla servers still
// answer getserversExt Quake3Arena queries.
func (e regEntry) matchGamename(want string) bool {
	if want == "" {
		return true
	}
	have := e.gamename
	if have == "" {
		have = "Quake3Arena"
	}
	return strings.EqualFold(have, want)
}
