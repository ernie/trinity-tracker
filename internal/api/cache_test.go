package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newCacheTestRouter() *Router {
	return &Router{respCache: newResponseCache()}
}

// Anonymous requests within the TTL get the cached body; the handler
// runs once.
func TestCached_HitWithinTTL(t *testing.T) {
	r := newCacheTestRouter()
	var calls atomic.Int32
	h := r.cached(time.Minute, func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"n":%d}`, calls.Load())
	})

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest("GET", "/api/test", nil))
		if w.Code != http.StatusOK || w.Body.String() != `{"n":1}` {
			t.Fatalf("request %d: got %d %q", i, w.Code, w.Body.String())
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Fatalf("request %d: Content-Type %q not replayed", i, ct)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("handler ran %d times, want 1", calls.Load())
	}
}

// Distinct query strings are distinct cache keys.
func TestCached_KeyIncludesQuery(t *testing.T) {
	r := newCacheTestRouter()
	var calls atomic.Int32
	h := r.cached(time.Minute, func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		w.Write([]byte(req.URL.RawQuery))
	})

	for _, q := range []string{"sort=frags", "sort=wins", "sort=frags"} {
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest("GET", "/api/test?"+q, nil))
		if w.Body.String() != q {
			t.Fatalf("query %q: got body %q", q, w.Body.String())
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("handler ran %d times, want 2", calls.Load())
	}
}

// An expired entry re-runs the handler.
func TestCached_ExpiryRerunsHandler(t *testing.T) {
	r := newCacheTestRouter()
	var calls atomic.Int32
	h := r.cached(time.Millisecond, func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		w.Write([]byte("x"))
	})

	h(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/test", nil))
	time.Sleep(5 * time.Millisecond)
	h(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/test", nil))
	if calls.Load() != 2 {
		t.Fatalf("handler ran %d times, want 2", calls.Load())
	}
}

// Credentialed requests bypass the cache in both directions: they never
// populate it and never read it. Validity is irrelevant — presence of
// the credential is the test.
func TestCached_CredentialBypass(t *testing.T) {
	for _, tc := range []struct {
		name    string
		addCred func(req *http.Request)
	}{
		{"authorization header", func(req *http.Request) {
			req.Header.Set("Authorization", "Bearer garbage")
		}},
		{"session cookie", func(req *http.Request) {
			req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "garbage"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newCacheTestRouter()
			var calls atomic.Int32
			h := r.cached(time.Minute, func(w http.ResponseWriter, req *http.Request) {
				fmt.Fprintf(w, "%d", calls.Add(1))
			})
			do := func(cred bool) string {
				req := httptest.NewRequest("GET", "/api/test", nil)
				if cred {
					tc.addCred(req)
				}
				w := httptest.NewRecorder()
				h(w, req)
				return w.Body.String()
			}

			// Credentialed requests never populate the cache: each runs
			// the handler.
			if got := do(true); got != "1" {
				t.Fatalf("credentialed #1: got %q", got)
			}
			if got := do(true); got != "2" {
				t.Fatalf("credentialed #2: got %q (cache populated by a credentialed request)", got)
			}
			// Anonymous misses (nothing was populated) and caches "3"...
			if got := do(false); got != "3" {
				t.Fatalf("anonymous: got %q", got)
			}
			// ...which a credentialed request must not read.
			if got := do(true); got != "4" {
				t.Fatalf("credentialed after anonymous: got %q (read the cached entry)", got)
			}
		})
	}
}

// Concurrent cold requests collapse into one handler execution.
func TestCached_SingleFlight(t *testing.T) {
	r := newCacheTestRouter()
	var calls atomic.Int32
	release := make(chan struct{})
	h := r.cached(time.Minute, func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		<-release
		w.Write([]byte("done"))
	})

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			h(w, httptest.NewRequest("GET", "/api/test", nil))
			if w.Body.String() != "done" {
				t.Errorf("got body %q", w.Body.String())
			}
		}()
	}
	// Let the goroutines pile up on the in-flight leader, then release.
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("handler ran %d times, want 1", calls.Load())
	}
}

// Non-200 responses reach the caller but are not cached.
func TestCached_ErrorsNotCached(t *testing.T) {
	r := newCacheTestRouter()
	var calls atomic.Int32
	h := r.cached(time.Minute, func(w http.ResponseWriter, req *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("recovered"))
	})

	w := httptest.NewRecorder()
	h(w, httptest.NewRequest("GET", "/api/test", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("first: got %d want 500", w.Code)
	}
	w = httptest.NewRecorder()
	h(w, httptest.NewRequest("GET", "/api/test", nil))
	if w.Code != http.StatusOK || w.Body.String() != "recovered" {
		t.Fatalf("second: got %d %q, want fresh 200", w.Code, w.Body.String())
	}
}

// The entry cap holds under hostile key cardinality.
func TestCached_EntryCapEnforced(t *testing.T) {
	r := newCacheTestRouter()
	h := r.cached(time.Minute, func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("x"))
	})

	for i := 0; i < maxCacheEntries+100; i++ {
		h(httptest.NewRecorder(), httptest.NewRequest("GET", fmt.Sprintf("/api/test?offset=%d", i), nil))
	}
	r.respCache.mu.Lock()
	n := len(r.respCache.entries)
	r.respCache.mu.Unlock()
	if n > maxCacheEntries {
		t.Fatalf("cache holds %d entries, cap is %d", n, maxCacheEntries)
	}
}
