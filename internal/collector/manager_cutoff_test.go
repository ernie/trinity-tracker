package collector

import (
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/config"
	"github.com/ernie/trinity-tracker/internal/domain"
)

// TestCutoffForPrefersPerServerOverGlobal covers the Bug 1 fix:
// when both a per-server cutoff and a global fallback exist, the
// per-server value must win. Otherwise the global aggregate can
// censor a same-second event on a different server during
// next-boot replay.
func TestCutoffForPrefersPerServerOverGlobal(t *testing.T) {
	perServerTS := time.Date(2026, 5, 11, 17, 11, 58, 0, time.UTC)
	globalTS := time.Date(2026, 5, 11, 17, 12, 8, 0, time.UTC) // censoring value

	m := &ServerManager{
		replayCutoff:  globalTS,
		replayCutoffs: map[int64]time.Time{42: perServerTS},
	}

	got := m.cutoffFor(&config.Q3Server{}, &domain.Server{ID: 42})

	if !got.Equal(perServerTS) {
		t.Errorf("cutoffFor = %v, want %v (per-server, not global)", got, perServerTS)
	}
}

// TestCutoffForFallsBackToGlobalWhenServerAbsent ensures the
// legacy single-watermark file (no per-server entries) still works:
// the global fallback applies to every server.
func TestCutoffForFallsBackToGlobalWhenServerAbsent(t *testing.T) {
	globalTS := time.Date(2026, 5, 11, 17, 12, 8, 0, time.UTC)

	m := &ServerManager{
		replayCutoff:  globalTS,
		replayCutoffs: nil, // pre-(per-server) watermark file
	}

	got := m.cutoffFor(&config.Q3Server{}, &domain.Server{ID: 99})

	if !got.Equal(globalTS) {
		t.Errorf("cutoffFor = %v, want %v (global fallback)", got, globalTS)
	}
}

// TestCutoffForFallsBackToLastMatchEndedAt covers the no-watermark
// case: cutoffFor should still honor the hub-side per-server
// LastMatchEndedAt as the next-priority hint.
func TestCutoffForFallsBackToLastMatchEndedAt(t *testing.T) {
	lastEnd := time.Date(2026, 5, 11, 16, 0, 0, 0, time.UTC)
	m := &ServerManager{} // no global, no per-server

	got := m.cutoffFor(
		&config.Q3Server{},
		&domain.Server{ID: 5, LastMatchEndedAt: &lastEnd},
	)

	if !got.Equal(lastEnd) {
		t.Errorf("cutoffFor = %v, want %v (LastMatchEndedAt)", got, lastEnd)
	}
}
