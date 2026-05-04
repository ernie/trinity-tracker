package directory

import (
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/storage"
)

func TestRegistryUpsertAndSweep(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	clock := func() time.Time { return now }
	r := newRegistry(15*time.Minute, 10, clock)

	addr := mustAddrPort(t, "10.0.0.1:27960")
	ok := r.Upsert(addr, map[string]string{
		"protocol":      "68",
		"gamename":      "baseq3",
		"clients":       "2",
		"sv_maxclients": "16",
		"gametype":      "4",
	}, 42)
	if !ok {
		t.Fatal("Upsert returned false on fresh insert")
	}
	if r.Len() != 1 {
		t.Fatalf("Len=%d, want 1", r.Len())
	}

	snap := r.Snapshot()
	if len(snap) != 1 || snap[0].protocol != 68 || snap[0].clients != 2 || snap[0].serverID != 42 {
		t.Errorf("snapshot: %+v", snap)
	}

	now = now.Add(20 * time.Minute)
	if removed := r.Sweep(); removed != 1 {
		t.Errorf("Sweep removed %d, want 1", removed)
	}
	if r.Len() != 0 {
		t.Errorf("Len after sweep = %d, want 0", r.Len())
	}
}

func TestRegistryRefreshExtendsExpiry(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	clock := func() time.Time { return now }
	r := newRegistry(15*time.Minute, 10, clock)

	addr := mustAddrPort(t, "10.0.0.1:27960")
	r.Upsert(addr, map[string]string{"protocol": "68", "clients": "1"}, 1)

	now = now.Add(10 * time.Minute)
	r.Upsert(addr, map[string]string{"protocol": "68", "clients": "2"}, 1)
	now = now.Add(10 * time.Minute)
	if removed := r.Sweep(); removed != 0 {
		t.Errorf("Sweep removed %d after refresh, want 0", removed)
	}
}

func TestRegistryCapacity(t *testing.T) {
	r := newRegistry(15*time.Minute, 2, nil)
	if !r.Upsert(mustAddrPort(t, "10.0.0.1:27960"), map[string]string{"protocol": "68", "clients": "1"}, 1) {
		t.Fatal("first upsert failed")
	}
	if !r.Upsert(mustAddrPort(t, "10.0.0.2:27960"), map[string]string{"protocol": "68", "clients": "1"}, 2) {
		t.Fatal("second upsert failed")
	}
	if r.Upsert(mustAddrPort(t, "10.0.0.3:27960"), map[string]string{"protocol": "68", "clients": "1"}, 3) {
		t.Error("third upsert should have failed at cap")
	}
	// Existing entry must still refresh.
	if !r.Upsert(mustAddrPort(t, "10.0.0.1:27960"), map[string]string{"protocol": "68", "clients": "5"}, 1) {
		t.Error("existing entry refresh failed at cap")
	}
}

// makeRow is a terse builder for storage.DirectoryRegistration test rows.
func makeRow(addr string, validatedAt, expiresAt time.Time) storage.DirectoryRegistration {
	return storage.DirectoryRegistration{
		Addr: addr, ServerID: 1, Protocol: 68,
		Gamename: "baseq3", Engine: "trinity-engine/0.4.2",
		Clients: 2, MaxClients: 16, Gametype: 4,
		ValidatedAt: validatedAt, ExpiresAt: expiresAt,
	}
}

func TestDecideRestoreFreshSnapshot(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	rows := []storage.DirectoryRegistration{
		makeRow("10.0.0.1:27960", now.Add(-time.Minute), now.Add(14*time.Minute)),
		makeRow("10.0.0.2:27961", now.Add(-2*time.Minute), now.Add(13*time.Minute)),
	}
	keep, clear := decideRestore(rows, 5*time.Minute, now)
	if clear {
		t.Fatal("clearAll = true on fresh snapshot")
	}
	if len(keep) != 2 {
		t.Fatalf("kept %d rows, want 2", len(keep))
	}
}

func TestDecideRestoreStaleSnapshotClears(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	rows := []storage.DirectoryRegistration{
		// Newest row is 10 minutes old — beyond the 5-minute window.
		makeRow("10.0.0.1:27960", now.Add(-10*time.Minute), now.Add(5*time.Minute)),
		makeRow("10.0.0.2:27961", now.Add(-12*time.Minute), now.Add(3*time.Minute)),
	}
	keep, clear := decideRestore(rows, 5*time.Minute, now)
	if !clear {
		t.Fatal("clearAll = false on stale snapshot")
	}
	if len(keep) != 0 {
		t.Errorf("kept %d rows on stale snapshot, want 0", len(keep))
	}
}

func TestDecideRestoreDropsExpiredRows(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	rows := []storage.DirectoryRegistration{
		// Snapshot itself is fresh (one row's validated_at is recent),
		// but this individual row's expires_at has already passed.
		makeRow("10.0.0.1:27960", now.Add(-30*time.Second), now.Add(-1*time.Second)),
		makeRow("10.0.0.2:27961", now.Add(-time.Minute), now.Add(14*time.Minute)),
	}
	keep, clear := decideRestore(rows, 5*time.Minute, now)
	if clear {
		t.Fatal("clearAll = true unexpectedly")
	}
	if len(keep) != 1 || keep[0].addr.String() != "10.0.0.2:27961" {
		t.Errorf("keep = %+v, want only 10.0.0.2", keep)
	}
}

func TestDecideRestoreEmpty(t *testing.T) {
	keep, clear := decideRestore(nil, 5*time.Minute, time.Now())
	if clear || len(keep) != 0 {
		t.Errorf("empty input: keep=%v clear=%v", keep, clear)
	}
}

func TestRegistryLoadAndSnapshotRoundtrip(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	r := newRegistry(15*time.Minute, 10, func() time.Time { return now })

	addr := mustAddrPort(t, "10.0.0.1:27960")
	r.Load([]regEntry{{
		addr: addr, protocol: 68, gamename: "baseq3", engine: "trinity-engine/0.4.2",
		clients: 2, maxClients: 16, gametype: 4,
		validatedAt: now.Add(-time.Minute), expiresAt: now.Add(14 * time.Minute),
		serverID: 42,
	}})
	if r.Len() != 1 {
		t.Fatalf("Len after Load = %d, want 1", r.Len())
	}
	persisted := r.toPersisted()
	if len(persisted) != 1 || persisted[0].Addr != "10.0.0.1:27960" || persisted[0].ServerID != 42 {
		t.Errorf("toPersisted = %+v", persisted)
	}
}

func TestRegistryLoadReplacesPriorEntries(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	r := newRegistry(15*time.Minute, 10, func() time.Time { return now })
	r.Upsert(mustAddrPort(t, "10.0.0.99:27960"),
		map[string]string{"protocol": "68", "clients": "1"}, 99)

	r.Load([]regEntry{{
		addr: mustAddrPort(t, "10.0.0.1:27960"), protocol: 68,
		validatedAt: now, expiresAt: now.Add(15 * time.Minute), serverID: 1,
	}})
	if r.Len() != 1 {
		t.Fatalf("Len = %d, want 1 (Load should replace, not merge)", r.Len())
	}
	if _, ok := r.entries[mustAddrPort(t, "10.0.0.99:27960")]; ok {
		t.Error("prior entry survived Load")
	}
}

func TestRegEntryGamenameMatch(t *testing.T) {
	cases := []struct {
		entryGN string
		want    string
		match   bool
	}{
		{"", "", true},
		{"baseq3", "", true},
		{"", "Quake3Arena", true}, // empty entry treated as default
		{"", "baseq3", false},
		{"baseq3", "baseq3", true},
		{"baseq3", "BASEQ3", true},
		{"baseq3", "missionpack", false},
	}
	for _, tc := range cases {
		e := regEntry{gamename: tc.entryGN}
		if got := e.matchGamename(tc.want); got != tc.match {
			t.Errorf("matchGamename(entry=%q want=%q) = %v, want %v",
				tc.entryGN, tc.want, got, tc.match)
		}
	}
}
