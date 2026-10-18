package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ernie/trinity-tracker/internal/storage"
)

func TestHandleGetLeaderboard_AsOfMalformed(t *testing.T) {
	r := &Router{}
	req := httptest.NewRequest("GET", "/api/stats/leaderboard?period=week&as_of=hello", nil)
	w := httptest.NewRecorder()
	r.handleGetLeaderboard(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

// Any well-formed as_of, far past or far future included, returns what
// the data says; on an empty database that is an empty list.
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
