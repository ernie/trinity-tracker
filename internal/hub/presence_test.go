package hub_test

import (
	"testing"

	"github.com/ernie/trinity-tracker/internal/hub"
)

func TestPresenceRecordAndLookup(t *testing.T) {
	p := hub.NewPresence()
	p.RecordJoin(1, 3, "guid-a")
	p.RecordJoin(1, 5, "guid-b")
	p.RecordJoin(2, 3, "guid-c")

	if got := p.Lookup(1, 3); got != "guid-a" {
		t.Errorf("Lookup(1,3) = %q, want guid-a", got)
	}
	if got := p.Lookup(1, 5); got != "guid-b" {
		t.Errorf("Lookup(1,5) = %q, want guid-b", got)
	}
	if got := p.Lookup(2, 3); got != "guid-c" {
		t.Errorf("Lookup(2,3) = %q, want guid-c", got)
	}
	if got := p.Lookup(1, 99); got != "" {
		t.Errorf("Lookup(1,99) = %q, want empty", got)
	}
}

func TestPresenceOverwrite(t *testing.T) {
	p := hub.NewPresence()
	p.RecordJoin(1, 3, "guid-a")
	p.RecordJoin(1, 3, "guid-b")
	if got := p.Lookup(1, 3); got != "guid-b" {
		t.Errorf("Lookup after overwrite = %q, want guid-b", got)
	}
}

func TestPresenceEmptyGUIDIsNoop(t *testing.T) {
	p := hub.NewPresence()
	p.RecordJoin(1, 3, "")
	if got := p.Lookup(1, 3); got != "" {
		t.Errorf("empty-guid join stored %q; want nothing", got)
	}
}

func TestPresenceLeaveClearsMatchingSlot(t *testing.T) {
	p := hub.NewPresence()
	p.RecordJoin(1, 3, "guid-a")
	p.RecordJoin(1, 5, "guid-b")
	p.RecordJoin(2, 3, "guid-a") // same guid on a different server
	p.RecordLeave(1, "guid-a")

	if got := p.Lookup(1, 3); got != "" {
		t.Errorf("Lookup(1,3) after leave = %q, want empty", got)
	}
	if got := p.Lookup(1, 5); got != "guid-b" {
		t.Errorf("unrelated slot cleared: Lookup(1,5) = %q", got)
	}
	if got := p.Lookup(2, 3); got != "guid-a" {
		t.Errorf("other-server slot cleared: Lookup(2,3) = %q", got)
	}
}

func TestPresenceClearServer(t *testing.T) {
	p := hub.NewPresence()
	p.RecordJoin(1, 3, "guid-a")
	p.RecordJoin(1, 5, "guid-b")
	p.RecordJoin(2, 3, "guid-c")
	p.Clear(1)
	if got := p.Lookup(1, 3); got != "" {
		t.Error("Clear(1) left slot 3 populated")
	}
	if got := p.Lookup(1, 5); got != "" {
		t.Error("Clear(1) left slot 5 populated")
	}
	if got := p.Lookup(2, 3); got != "guid-c" {
		t.Error("Clear(1) also cleared server 2")
	}
}
