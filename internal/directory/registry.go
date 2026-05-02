package directory

import (
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
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
