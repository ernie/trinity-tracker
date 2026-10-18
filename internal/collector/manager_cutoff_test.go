package collector

import (
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/config"
	"github.com/ernie/trinity-tracker/internal/domain"
)

// Each tailer's cutoff comes from the publisher's per-RemoteServerID
// entry, so another server's activity can't advance it.
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

// A server with no publisher entry yet (newly added, or idle since the
// last watermark write) falls back to its LastMatchEndedAt, the same
// answer the hub gives in normal operation.
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
