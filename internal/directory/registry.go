package directory

import (
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ernie/trinity-tracker/internal/storage"
)

// regEntry is one validated server. Each new infoResponse pushes
// expiresAt forward; the sweep removes entries past it.
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

// registry holds every validated server. At maxServers it refuses new
// addresses but still refreshes existing ones.
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

// Upsert records or refreshes a validated entry. It returns false only
// for a new address when the registry is full.
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

// Snapshot returns a copy of the registry to filter outside the lock.
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

// Len returns the number of registered servers.
func (r *registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}

// Load replaces the registry's contents, to restore a saved snapshot at
// startup. The caller filters the entries first (decideRestore).
func (r *registry) Load(entries []regEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = make(map[netip.AddrPort]*regEntry, len(entries))
	for i := range entries {
		e := entries[i]
		r.entries[e.addr] = &e
	}
}

// decideRestore filters a saved snapshot for restoring.
//
// If the newest validated_at is older than freshness, the hub was down
// longer than a routine restart, and the whole snapshot is dropped
// (clearAll) to show an empty list over stale data. Otherwise rows past
// their own expires_at are dropped.
//
// Restored entries skip the membership gate: after a hub restart,
// remote collectors take seconds to reconnect, so the gate is briefly
// empty for them. A source deprovisioned within the freshness window
// stays listed only until its entry expires, as it would in memory.
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

// toPersisted converts the entries for storage at graceful shutdown.
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
// A server without fs_game sends no gamename, which counts as
// "Quake3Arena", as in dpmaster, so stock servers answer getserversExt
// Quake3Arena queries.
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
