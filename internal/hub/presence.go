package hub

import "sync"

// PresenceEntry is per-slot state the hub uses to enrich UDP
// statusResponse rows. The counters are this match's awards:
// Writer.EnrichEvent increments them and Presence.ResetCounters zeroes
// them at match start.
type PresenceEntry struct {
	GUID  string
	Model string
	IsBot bool
	IsVR  bool
	Skill float64

	Impressives     int
	Excellents      int
	Humiliations    int
	Defends         int
	Captures        int
	Assists         int
	FlagReturns     int // CTF and One Flag CTF: returns a player made this match
	SkullsDelivered int // Harvester: cumulative deliveries this match (drives the live skull medal)
}

// Presence maps (serverID, clientNum) → entry, fed by
// player_join / player_leave events.
type Presence struct {
	mu     sync.RWMutex
	bySlot map[presenceKey]PresenceEntry
}

type presenceKey struct {
	serverID  int64
	clientNum int
}

// NewPresence returns an empty presence tracker.
func NewPresence() *Presence {
	return &Presence{bySlot: make(map[presenceKey]PresenceEntry)}
}

// RecordJoin stores entry for the slot, replacing any stale one. Bots
// that share a clean name, and so a synthetic GUID, stay apart by slot.
func (p *Presence) RecordJoin(serverID int64, clientNum int, entry PresenceEntry) {
	if entry.GUID == "" {
		return
	}
	p.mu.Lock()
	p.bySlot[presenceKey{serverID, clientNum}] = entry
	p.mu.Unlock()
}

// RecordLeave clears the slot if it still holds guid, so a late leave
// can't clear a slot that has a new player.
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

// UpdateClientUserinfo applies a ClientUserinfoChanged (model, VR) to
// the slot's entry if it still holds guid, so a late update can't
// overwrite a new player.
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

// HumanCount returns the number of human entries for serverID. The
// activity notifier fires when it moves between 0 and 1.
func (p *Presence) HumanCount(serverID int64) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	n := 0
	for k, v := range p.bySlot {
		if k.serverID == serverID && !v.IsBot {
			n++
		}
	}
	return n
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

// ResetCounters zeroes the per-match counters on every entry for
// serverID at match start, so the live card starts each match at 0.
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
		v.FlagReturns = 0
		v.SkullsDelivered = 0
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
	AwardFlagReturn
	AwardSkullDelivered
)

// IncrementByGUID bumps the named counter by 1 on whichever slot on
// serverID currently holds guid. No-op if no slot matches.
func (p *Presence) IncrementByGUID(serverID int64, guid string, award Award) {
	p.IncrementByGUIDBy(serverID, guid, award, 1)
}

// IncrementByGUIDBy is IncrementByGUID with an explicit count, for
// multi-award events like a multi-skull SkullScore.
func (p *Presence) IncrementByGUIDBy(serverID int64, guid string, award Award, count int) {
	if guid == "" || count <= 0 {
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
			v.Impressives += count
		case AwardExcellent:
			v.Excellents += count
		case AwardHumiliation:
			v.Humiliations += count
		case AwardDefend:
			v.Defends += count
		case AwardCapture:
			v.Captures += count
		case AwardAssist:
			v.Assists += count
		case AwardFlagReturn:
			v.FlagReturns += count
		case AwardSkullDelivered:
			v.SkullsDelivered += count
		}
		p.bySlot[k] = v
		return
	}
}
