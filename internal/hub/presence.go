package hub

import "sync"

// PresenceEntry is per-slot state the hub uses to enrich UDP
// statusResponse rows. Counters hold per-match awards incremented by
// Writer.EnrichEvent and reset by Presence.ResetCounters on match
// start.
type PresenceEntry struct {
	GUID  string
	Model string
	IsBot bool
	IsVR  bool
	Skill float64

	Impressives  int
	Excellents   int
	Humiliations int
	Defends      int
	Captures     int
	Assists      int
}

// Presence maps (serverID, clientNum) → entry, fed by
// player_join / player_leave events.
type Presence struct {
	mu     sync.RWMutex
	bySlot map[presenceKey]PresenceEntry // (serverID, clientNum) → entry
}

type presenceKey struct {
	serverID  int64
	clientNum int
}

// NewPresence returns an empty presence tracker.
func NewPresence() *Presence {
	return &Presence{bySlot: make(map[presenceKey]PresenceEntry)}
}

// RecordJoin remembers (serverID, clientNum) → entry. Overwrites any
// stale entry for the same slot. Two bots that happen to share a
// clean name (and therefore a synthetic GUID) are distinguished by
// their slot, so no cross-slot cleanup happens here.
func (p *Presence) RecordJoin(serverID int64, clientNum int, entry PresenceEntry) {
	if entry.GUID == "" {
		return
	}
	p.mu.Lock()
	p.bySlot[presenceKey{serverID, clientNum}] = entry
	p.mu.Unlock()
}

// RecordLeave clears the specific (serverID, clientNum) slot if its
// current GUID matches. The guid check guards against clearing a slot
// that's already been reassigned to a different player when a stale
// leave arrives.
func (p *Presence) RecordLeave(serverID int64, clientNum int, guid string) {
	if guid == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	k := presenceKey{serverID, clientNum}
	if v, ok := p.bySlot[k]; ok && v.GUID == guid {
		delete(p.bySlot, k)
	}
}

// UpdateClientUserinfo applies a ClientUserinfoChanged (model, vr) to
// an existing presence entry. No-op if the slot has no entry or its
// GUID doesn't match — a stale update after the slot has been
// reassigned must not overwrite the new occupant.
func (p *Presence) UpdateClientUserinfo(serverID int64, clientNum int, guid, model string, isVR bool) {
	if guid == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	k := presenceKey{serverID, clientNum}
	entry, ok := p.bySlot[k]
	if !ok || entry.GUID != guid {
		return
	}
	entry.Model = model
	entry.IsVR = isVR
	p.bySlot[k] = entry
}

func (p *Presence) Lookup(serverID int64, clientNum int) (PresenceEntry, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	entry, ok := p.bySlot[presenceKey{serverID, clientNum}]
	return entry, ok
}

func (p *Presence) Clear(serverID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k := range p.bySlot {
		if k.serverID == serverID {
			delete(p.bySlot, k)
		}
	}
}

// ResetCounters zeroes the per-match counter fields on every entry
// for serverID, keeping identity fields intact. Called at match start
// so the live card starts each match at 0-0-0.
func (p *Presence) ResetCounters(serverID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, v := range p.bySlot {
		if k.serverID != serverID {
			continue
		}
		v.Impressives = 0
		v.Excellents = 0
		v.Humiliations = 0
		v.Defends = 0
		v.Captures = 0
		v.Assists = 0
		p.bySlot[k] = v
	}
}

// Award enumerates which per-match counter to bump.
type Award int

const (
	AwardImpressive Award = iota
	AwardExcellent
	AwardHumiliation
	AwardDefend
	AwardCapture
	AwardAssist
)

// IncrementByGUID bumps the named counter on whichever slot on
// serverID currently holds guid. No-op if no entry matches (e.g. the
// event arrived before the corresponding player_join, or for the
// server-level no-killer victim in some award variants).
func (p *Presence) IncrementByGUID(serverID int64, guid string, award Award) {
	if guid == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, v := range p.bySlot {
		if k.serverID != serverID || v.GUID != guid {
			continue
		}
		switch award {
		case AwardImpressive:
			v.Impressives++
		case AwardExcellent:
			v.Excellents++
		case AwardHumiliation:
			v.Humiliations++
		case AwardDefend:
			v.Defends++
		case AwardCapture:
			v.Captures++
		case AwardAssist:
			v.Assists++
		}
		p.bySlot[k] = v
		return
	}
}
