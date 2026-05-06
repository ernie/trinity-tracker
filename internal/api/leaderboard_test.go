package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ernie/trinity-tracker/internal/storage"
)

// Malformed as_of must 400 before the store is touched.
func TestHandleGetLeaderboard_AsOfMalformed(t *testing.T) {
	r := &Router{}
	req := httptest.NewRequest("GET", "/api/stats/leaderboard?period=week&as_of=hello", nil)
	w := httptest.NewRecorder()
	r.handleGetLeaderboard(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

// A well-formed as_of (RFC3339) is accepted and reaches the store —
// against an empty DB, that produces an empty entries array, not a
// 500. Far-future and far-past anchors must succeed for the same
// reason: we don't police range, we just return what the data says.
func TestHandleGetLeaderboard_AsOfAccepted(t *testing.T) {
	store, err := storage.New(filepath.Join(t.TempDir(), "trinity.db"))
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	defer store.Close()
	r := &Router{store: store}

	for _, asOf := range []string{
		"2026-05-06T20:00:00Z", // sensible
		"2099-01-01T00:00:00Z", // far future
		"1999-01-01T00:00:00Z", // far past
	} {
		t.Run(asOf, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/stats/leaderboard?period=week&as_of="+asOf, nil)
			w := httptest.NewRecorder()
			r.handleGetLeaderboard(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("got %d, want 200; body=%s", w.Code, w.Body.String())
			}
		})
	}
}
