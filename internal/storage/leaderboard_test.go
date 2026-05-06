package storage

import (
	"testing"
	"time"
)

// getTimePeriodBounds is the only place leaderboard windows are
// computed; pinning asOf makes the result reproducible for snapshot
// links. These tests lock that contract in.
func TestGetTimePeriodBounds_AsOfPinsTheUpperBound(t *testing.T) {
	pin := time.Date(2026, 5, 6, 20, 0, 0, 0, time.UTC)

	cases := []struct {
		period    string
		wantStart time.Time
	}{
		{"day", pin.Add(-24 * time.Hour)},
		{"week", pin.Add(-7 * 24 * time.Hour)},
		{"month", pin.Add(-30 * 24 * time.Hour)},
		{"year", pin.Add(-365 * 24 * time.Hour)},
	}
	for _, tc := range cases {
		t.Run(tc.period, func(t *testing.T) {
			start, end := getTimePeriodBounds(tc.period, pin)
			if !start.Equal(tc.wantStart) {
				t.Errorf("start: got %v, want %v", start, tc.wantStart)
			}
			if !end.Equal(pin) {
				t.Errorf("end: got %v, want %v", end, pin)
			}
		})
	}
}

// Zero-value asOf falls back to time.Now(). The exact "now" is racy
// to assert against, so we just check the bounds straddle the call.
func TestGetTimePeriodBounds_ZeroAsOfIsLive(t *testing.T) {
	before := time.Now()
	start, end := getTimePeriodBounds("week", time.Time{})
	after := time.Now()

	if end.Before(before) || end.After(after) {
		t.Errorf("end %v not in [%v,%v]", end, before, after)
	}
	expectedStart := end.Add(-7 * 24 * time.Hour)
	if !start.Equal(expectedStart) {
		t.Errorf("start: got %v, want %v", start, expectedStart)
	}
}

// "all" returns a zero start and a far-future end (to bypass any
// time-based WHERE clauses). asOf still anchors the offset.
func TestGetTimePeriodBounds_AllSpansEverything(t *testing.T) {
	pin := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	start, end := getTimePeriodBounds("all", pin)
	if !start.IsZero() {
		t.Errorf("start: got %v, want zero", start)
	}
	if !end.After(pin.Add(99 * 365 * 24 * time.Hour)) {
		t.Errorf("end %v should be far past %v", end, pin)
	}
}
