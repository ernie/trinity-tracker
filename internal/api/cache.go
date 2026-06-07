package api

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// responseCache is a small TTL cache for anonymous GET responses;
// singleflight collapses a read spike into one handler execution.
type responseCache struct {
	mu      sync.Mutex
	entries map[string]*cachedResponse
	group   singleflight.Group
}

// maxCacheEntries keeps a crawler's arbitrary query strings from
// ballooning the map.
const maxCacheEntries = 1024

type cachedResponse struct {
	status  int
	header  http.Header
	body    []byte
	expires time.Time
}

func newResponseCache() *responseCache {
	return &responseCache{entries: make(map[string]*cachedResponse)}
}

// responseRecorder captures a handler's response for replay. Plain
// ResponseWriter only — the cached handlers never need Flusher/Hijacker.
type responseRecorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *responseRecorder) Header() http.Header { return r.header }

func (r *responseRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(b)
}

// cached wraps a GET handler with the response cache. Credentialed
// requests bypass in both directions — never read, never populate — so
// a viewer-specific response can't leak. Credential presence suffices:
// a garbage one just means an uncached (correct) response.
func (r *Router) cached(ttl time.Duration, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "" {
			next(w, req)
			return
		}
		if c, err := req.Cookie(sessionCookieName); err == nil && c.Value != "" {
			next(w, req)
			return
		}

		// RawQuery uncanonicalized: a reordered-param duplicate just
		// costs an extra entry.
		key := req.URL.Path + "?" + req.URL.RawQuery

		if resp := r.respCache.get(key); resp != nil {
			resp.replay(w)
			return
		}

		v, _, _ := r.respCache.group.Do(key, func() (interface{}, error) {
			// A prior flight may have populated the entry since the
			// miss above.
			if resp := r.respCache.get(key); resp != nil {
				return resp, nil
			}
			rec := &responseRecorder{header: make(http.Header)}
			// Detached so a leader disconnect mid-query can't poison
			// the shared result for every waiter.
			detached := req.WithContext(context.WithoutCancel(req.Context()))
			next(rec, detached)
			resp := &cachedResponse{
				status:  rec.status,
				header:  rec.header,
				body:    rec.body.Bytes(),
				expires: time.Now().Add(ttl),
			}
			// Non-200s reach current waiters but are never cached.
			if rec.status == http.StatusOK {
				r.respCache.put(key, resp)
			}
			return resp, nil
		})
		v.(*cachedResponse).replay(w)
	}
}

func (c *responseCache) get(key string) *cachedResponse {
	c.mu.Lock()
	defer c.mu.Unlock()
	resp, ok := c.entries[key]
	if !ok || time.Now().After(resp.expires) {
		return nil
	}
	return resp
}

func (c *responseCache) put(key string, resp *cachedResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= maxCacheEntries {
		c.evictLocked()
	}
	c.entries[key] = resp
}

// evictLocked drops expired entries, then arbitrary ones until under
// the cap — entries are short-lived, fancier isn't worth the bookkeeping.
func (c *responseCache) evictLocked() {
	now := time.Now()
	for k, v := range c.entries {
		if now.After(v.expires) {
			delete(c.entries, k)
		}
	}
	for k := range c.entries {
		if len(c.entries) < maxCacheEntries {
			break
		}
		delete(c.entries, k)
	}
}

func (resp *cachedResponse) replay(w http.ResponseWriter) {
	for k, vs := range resp.header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.status)
	w.Write(resp.body)
}
