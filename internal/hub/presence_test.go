package hub_test

import (
	"testing"

	"github.com/ernie/trinity-tracker/internal/hub"
)

func entry(guid string) hub.PresenceEntry { return hub.PresenceEntry{GUID: guid} }

func TestPresenceRecordAndLookup(t *testing.T) {
	p := hub.NewPresence()
	p.RecordJoin(1, 3, entry("guid-a"))
	p.RecordJoin(1, 5, entry("guid-b"))
	p.RecordJoin(2, 3, entry("guid-c"))

	if got, ok := p.Lookup(1, 3); !ok || got.GUID != "guid-a" {
		t.Errorf("Lookup(1,3) = %+v ok=%v, want guid-a", got, ok)
	}
	if got, ok := p.Lookup(1, 5); !ok || got.GUID != "guid-b" {
		t.Errorf("Lookup(1,5) = %+v ok=%v, want guid-b", got, ok)
	}
	if got, ok := p.Lookup(2, 3); !ok || got.GUID != "guid-c" {
		t.Errorf("Lookup(2,3) = %+v ok=%v, want guid-c", got, ok)
	}
	if _, ok := p.Lookup(1, 99); ok {
		t.Error("Lookup(1,99) ok=true, want false")
	}
}

func TestPresenceOverwrite(t *testing.T) {
	p := hub.NewPresence()
	p.RecordJoin(1, 3, entry("guid-a"))
	p.RecordJoin(1, 3, entry("guid-b"))
	if got, _ := p.Lookup(1, 3); got.GUID != "guid-b" {
		t.Errorf("Lookup after overwrite = %q, want guid-b", got.GUID)
	}
}

func TestPresenceEmptyGUIDIsNoop(t *testing.T) {
	p := hub.NewPresence()
	p.RecordJoin(1, 3, entry(""))
	if _, ok := p.Lookup(1, 3); ok {
		t.Error("empty-guid join stored; want nothing")
	}
}

// Two bots with the same clean name (and thus the same synthetic
// BOT:<name> GUID) can coexist on the same server at different slots.
// Leave on one slot must not clobber the other.
func TestPresenceDuplicateGUIDAcrossSlots(t *testing.T) {
	p := hub.NewPresence()
	p.RecordJoin(1, 0, entry("BOT:Sarge"))
	p.RecordJoin(1, 3, entry("BOT:Sarge"))

	if got, ok := p.Lookup(1, 0); !ok || got.GUID != "BOT:Sarge" {
		t.Errorf("slot 0 = %+v ok=%v, want BOT:Sarge", got, ok)
	}
	if got, ok := p.Lookup(1, 3); !ok || got.GUID != "BOT:Sarge" {
		t.Errorf("slot 3 = %+v ok=%v, want BOT:Sarge", got, ok)
	}

	p.RecordLeave(1, 0, "BOT:Sarge")
	if _, ok := p.Lookup(1, 0); ok {
		t.Error("slot 0 still present after targeted leave")
	}
	if got, ok := p.Lookup(1, 3); !ok || got.GUID != "BOT:Sarge" {
		t.Errorf("slot 3 clobbered by slot 0 leave: got=%+v ok=%v", got, ok)
	}
}

func TestPresenceLeaveClearsMatchingSlot(t *testing.T) {
	p := hub.NewPresence()
	p.RecordJoin(1, 3, entry("guid-a"))
	p.RecordJoin(1, 5, entry("guid-b"))
	p.RecordJoin(2, 3, entry("guid-a"))
	p.RecordLeave(1, 3, "guid-a")

	if _, ok := p.Lookup(1, 3); ok {
		t.Error("Lookup(1,3) after leave still present")
	}
	if got, _ := p.Lookup(1, 5); got.GUID != "guid-b" {
		t.Errorf("unrelated slot cleared: Lookup(1,5) = %q", got.GUID)
	}
	if got, _ := p.Lookup(2, 3); got.GUID != "guid-a" {
		t.Errorf("other-server slot cleared: Lookup(2,3) = %q", got.GUID)
	}
}

func TestPresenceLeaveGUIDMismatchNoOp(t *testing.T) {
	p := hub.NewPresence()
	p.RecordJoin(1, 0, entry("new-guid"))
	p.RecordLeave(1, 0, "stale-guid")
	if got, ok := p.Lookup(1, 0); !ok || got.GUID != "new-guid" {
		t.Errorf("stale leave cleared reassigned slot: got=%+v ok=%v", got, ok)
	}
}

func TestPresenceUpdateClientUserinfo(t *testing.T) {
	p := hub.NewPresence()
	p.RecordJoin(1, 0, hub.PresenceEntry{GUID: "H", Model: "sarge", IsVR: false})

	p.UpdateClientUserinfo(1, 0, "H", "doom", true)
	got, _ := p.Lookup(1, 0)
	if got.Model != "doom" || !got.IsVR {
		t.Errorf("update applied wrong: %+v", got)
	}

	p.RecordJoin(1, 0, hub.PresenceEntry{GUID: "H2", Model: "keel"})
	p.UpdateClientUserinfo(1, 0, "H", "leaked", false)
	got, _ = p.Lookup(1, 0)
	if got.Model != "keel" {
		t.Errorf("stale update overwrote new occupant: %+v", got)
	}

	p.UpdateClientUserinfo(9, 99, "X", "whatever", false)
	if _, ok := p.Lookup(9, 99); ok {
		t.Error("UpdateClientUserinfo created an entry from nothing")
	}
}

func TestPresenceClearServer(t *testing.T) {
	p := hub.NewPresence()
	p.RecordJoin(1, 3, entry("guid-a"))
	p.RecordJoin(1, 5, entry("guid-b"))
	p.RecordJoin(2, 3, entry("guid-c"))
	p.Clear(1)
	if _, ok := p.Lookup(1, 3); ok {
		t.Error("Clear(1) left slot 3 populated")
	}
	if _, ok := p.Lookup(1, 5); ok {
		t.Error("Clear(1) left slot 5 populated")
	}
	if got, _ := p.Lookup(2, 3); got.GUID != "guid-c" {
		t.Error("Clear(1) also cleared server 2")
	}
}
