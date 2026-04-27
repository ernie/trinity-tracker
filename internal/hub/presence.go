package hub

import "sync"

// Presence maps (serverID, clientNum) → GUID for currently-connected
// players across every collector feeding this hub. The hub poller
// consults it to enrich UDP statusResponse rows — which carry only
// name, ping, score, and a slot number — with identity.
//
// Fed by the writer's dispatch loop: PlayerJoinData puts an entry in,
// PlayerLeaveData (by GUID) clears matching entries on the same
// server. A process-wide lookup on player_guids + users backs
// IsVerified / IsAdmin when the poller needs them.
type Presence struct {
	mu      sync.RWMutex
	bySlot  map[presenceKey]string // (serverID, clientNum) → guid
}

type presenceKey struct {
	serverID  int64
	clientNum int
}

// NewPresence returns an empty presence tracker.
func NewPresence() *Presence {
	return &Presence{bySlot: make(map[presenceKey]string)}
}

// RecordJoin remembers (serverID, clientNum) → guid. Overwrites any
// stale entry for the same slot (a replay or restart may re-arrive).
func (p *Presence) RecordJoin(serverID int64, clientNum int, guid string) {
	if guid == "" {
		return
	}
	p.mu.Lock()
	p.bySlot[presenceKey{serverID, clientNum}] = guid
	p.mu.Unlock()
}

// RecordLeave drops every slot entry on serverID whose value matches
// guid. PlayerLeaveData doesn't carry the clientNum (Q3 logs
// disconnect by id but we don't thread it through the envelope yet),
// so we match by GUID. In practice a human only occupies one slot at
// a time, so this iterates at most once.
func (p *Presence) RecordLeave(serverID int64, guid string) {
	if guid == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, v := range p.bySlot {
		if k.serverID == serverID && v == guid {
			delete(p.bySlot, k)
		}
	}
}

// Lookup returns the GUID at (serverID, clientNum) or "" if unknown.
func (p *Presence) Lookup(serverID int64, clientNum int) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.bySlot[presenceKey{serverID, clientNum}]
}

// Clear removes all entries for serverID (e.g. on ServerStartup /
// ServerShutdown events).
func (p *Presence) Clear(serverID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k := range p.bySlot {
		if k.serverID == serverID {
			delete(p.bySlot, k)
		}
	}
}
