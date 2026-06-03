package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleLiveRedirects(t *testing.T) {
	tr := newTestRouter(t)
	ctx := context.Background()
	_, ownerID := tr.loginAs(t, "owner", false)
	if err := tr.store.CreateSource(ctx, "remote-1", true, &ownerID); err != nil {
		t.Fatalf("create source: %v", err)
	}
	// demo_base_url is only ever set via a collector heartbeat. Seed a
	// trailing slash to exercise the TrimRight in handleLive.
	if err := tr.store.TouchSourceHeartbeat(ctx, "remote-1", time.Now().UTC(), "", "https://remote-1.example.com/"); err != nil {
		t.Fatalf("seed heartbeat: %v", err)
	}

	req := httptest.NewRequest("GET", "/tv/remote-1/ffa/stream", nil)
	w := httptest.NewRecorder()
	tr.r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	if got, want := w.Header().Get("Location"), "https://remote-1.example.com/tv/ffa"; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestHandleLiveRejectsInvalidKey(t *testing.T) {
	tr := newTestRouter(t)
	ctx := context.Background()
	_, ownerID := tr.loginAs(t, "owner", false)
	if err := tr.store.CreateSource(ctx, "remote-1", true, &ownerID); err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := tr.store.TouchSourceHeartbeat(ctx, "remote-1", time.Now().UTC(), "", "https://remote-1.example.com/"); err != nil {
		t.Fatalf("seed heartbeat: %v", err)
	}

	// A key outside [A-Za-z0-9._-] must be refused before it's concatenated raw
	// into the redirect Location (defense-in-depth — the collector relay 404s
	// such keys anyway, but the hub shouldn't emit a malformed 302).
	for _, key := range []string{"ffa@home", "ffa~x", "ffa:9999"} {
		req := httptest.NewRequest("GET", "/tv/remote-1/"+key+"/stream", nil)
		w := httptest.NewRecorder()
		tr.r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("key %q: status = %d, want 404 (Location=%q)", key, w.Code, w.Header().Get("Location"))
		}
	}
}

func TestHandleLiveUnknownSource404(t *testing.T) {
	tr := newTestRouter(t)
	req := httptest.NewRequest("GET", "/tv/ghost/ffa/stream", nil)
	w := httptest.NewRecorder()
	tr.r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}
