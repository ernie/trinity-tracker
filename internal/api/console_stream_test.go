package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/console"
)

func TestConsoleStreamLocal(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "hubadmin", true)
	nobodyTok, _ := tr.loginAs(t, "nobody", false)
	_, ownerID := tr.loginAs(t, "alice", false)
	seedConsoleWorld(t, tr, ownerID)

	reg := console.NewRegistry()
	ring := reg.Ring("ffa")
	ring.SetTapUp(true)
	ring.Append("scroll-line")
	tr.r.SetConsoleRegistry(reg)

	srv := httptest.NewServer(tr.r)
	defer srv.Close()

	get := func(token, query string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("GET", srv.URL+"/api/console/stream?"+query, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		return resp
	}

	// Authz surface.
	if resp := get("", "source=hub-q3&key=ffa"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", resp.StatusCode)
	}
	if resp := get(nobodyTok, "source=hub-q3&key=ffa"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("unprivileged: %d", resp.StatusCode)
	}
	if resp := get(adminTok, "source=hub-q3&key=nope"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown server: %d", resp.StatusCode)
	}

	// Streaming: status event, scrollback, then a live line.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET",
		srv.URL+"/api/console/stream?source=hub-q3&key=ffa", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream GET: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type %q", ct)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		ring.Append("live-line")
	}()

	var sawStatus, sawScroll, sawLive bool
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.Contains(line, `"tap_up":true`):
			sawStatus = true
		case strings.Contains(line, "scroll-line"):
			sawScroll = true
		case strings.Contains(line, "live-line"):
			sawLive = true
		}
		if sawStatus && sawScroll && sawLive {
			break
		}
	}
	if !sawStatus || !sawScroll || !sawLive {
		t.Errorf("status=%v scroll=%v live=%v", sawStatus, sawScroll, sawLive)
	}
}

func TestConsoleStreamUnavailable(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "hubadmin", true)
	_, ownerID := tr.loginAs(t, "alice", false)
	seedConsoleWorld(t, tr, ownerID)
	tr.r.SetConsoleRegistry(console.NewRegistry()) // no ring for ffa

	w := tr.do("GET", "/api/console/stream?source=hub-q3&key=ffa", "", adminTok)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("no ring: want 503, got %d", w.Code)
	}

	// Remote source without a relay: 501.
	w = tr.do("GET", "/api/console/stream?source=alice-q3&key=ctf", "", adminTok)
	if w.Code != http.StatusNotImplemented {
		t.Errorf("no relay: want 501, got %d", w.Code)
	}
}
