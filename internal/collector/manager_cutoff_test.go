package collector

import (
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/config"
	"github.com/ernie/trinity-tracker/internal/domain"
)

// TestCutoffForUsesPerServerWhenPresent covers the Bug 1 fix: each
// tailer's cutoff comes from the publisher's per-RemoteServerID
// entry, not from a single global value that any other server's
// activity could advance.
func TestCutoffForUsesPerServerWhenPresent(t *testing.T) {
	perServerTS := time.Date(2026, 5, 11, 17, 11, 58, 0, time.UTC)

	m := &ServerManager{
		replayCutoffs: map[int64]time.Time{42: perServerTS},
	}

	got := m.cutoffFor(&config.Q3Server{}, &domain.Server{ID: 42})

	if !got.Equal(perServerTS) {
		t.Errorf("cutoffFor = %v, want %v (per-server)", got, perServerTS)
	}
}

// TestCutoffForFallsBackToLastMatchEndedAt covers the "no
// publisher-side entry for this server yet" case: a newly added
// server, or one with no published activity since the last
// watermark write. Falling through to the hub-side per-server
// LastMatchEndedAt is correct: it's the same answer the hub gives
// during normal operation, just sourced from the matches table.
func TestCutoffForFallsBackToLastMatchEndedAt(t *testing.T) {
	lastEnd := time.Date(2026, 5, 11, 16, 0, 0, 0, time.UTC)
	m := &ServerManager{} // no per-server entries

	got := m.cutoffFor(
		&config.Q3Server{},
		&domain.Server{ID: 5, LastMatchEndedAt: &lastEnd},
	)

	if !got.Equal(lastEnd) {
		t.Errorf("cutoffFor = %v, want %v (LastMatchEndedAt)", got, lastEnd)
	}
}
