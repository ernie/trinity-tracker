package directory

import (
	"testing"
	"time"
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
