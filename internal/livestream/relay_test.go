// internal/livestream/relay_test.go
package livestream

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRelayServerRejects404(t *testing.T) {
	srv := NewRelayServer(NewRegistry())
	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"empty key", "GET", "/tv/"},
		{"nested path", "GET", "/tv/a/b"},
		{"outside prefix", "GET", "/other/m1"},
		{"non-GET method", "POST", "/tv/m1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Fatalf("%s %s = %d, want 404", tc.method, tc.path, w.Code)
			}
		})
	}
}

func TestRelayServerNotLiveYet(t *testing.T) {
	// A well-formed request for a server key with no current buffer
	// (inter-match gap) must signal "retry", not a hard 404.
	srv := NewRelayServer(NewRegistry())
	req := httptest.NewRequest("GET", "/tv/ffa", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After header on 503")
	}
}

func TestRelayServerStreamsEndedBuffer(t *testing.T) {
	reg := NewRegistry()
	clk := &fakeClock{now: time.Unix(0, 0)}
	b := NewBuffer([]byte("HDR"), 10*time.Second)
	b.clk = clk
	for i, at := range []int64{0, 3, 6, 9} {
		clk.now = time.Unix(at, 0)
		b.Append(Segment{KeyframeServerTime: int32(i * 3000), Payload: []byte{byte('A' + i)}})
	}
	b.End()
	clk.now = time.Unix(14, 0)
	reg.Set("m1", b)
	reg.Close("m1") // single session, no next: let the relay finish after this buffer

	srv := NewRelayServer(reg)
	srv.tick = time.Second // deterministic with the fake clock

	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/tv/m1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("content-type = %q", ct)
	}
	if xab := resp.Header.Get("X-Accel-Buffering"); xab != "no" {
		t.Fatalf("X-Accel-Buffering = %q, want no", xab)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.HasPrefix(body, []byte("HDR")) {
		t.Fatalf("body missing HDR prefix: %v", body)
	}
}

func TestEndToEndFixtureStream(t *testing.T) {
	// Build a fixture live stream in memory.
	var raw bytes.Buffer
	raw.Write(StreamHeader{SvFps: 20, MaxClients: 8, MapName: "q3dm6", Timestamp: "2026-05-31"}.Marshal())
	for i := 0; i < 4; i++ {
		raw.Write(Segment{KeyframeServerTime: int32(i * 3000), Payload: []byte{byte('A' + i)}}.Marshal())
	}
	raw.WriteString(endMagic)

	// Parse header, build buffer with a fake clock, pump segments.
	hdr, err := ParseStreamHeader(&raw)
	if err != nil {
		t.Fatalf("ParseStreamHeader: %v", err)
	}
	clk := &fakeClock{now: time.Unix(0, 0)}
	b := NewBuffer(hdr.Marshal(), 10*time.Second)
	b.clk = clk
	// Pump the remaining segment bytes; arrivals all stamped at t=0 (then ended).
	if err := ConsumeSegments(&raw, b); err != nil {
		t.Fatalf("ConsumeSegments: %v", err)
	}
	clk.now = time.Unix(14, 0) // all four segments are now >=10s old

	reg := NewRegistry()
	reg.Set("m1", b)
	reg.Close("m1") // single session, no next: let the relay finish after this buffer
	srv := NewRelayServer(reg)
	srv.tick = time.Second
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/tv/m1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// Header must round-trip out of the relayed bytes.
	got, err := ParseStreamHeader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("relayed header parse: %v", err)
	}
	if got != hdr {
		t.Fatalf("relayed header = %+v, want %+v", got, hdr)
	}
}

func TestRegistrySetGetRemove(t *testing.T) {
	reg := NewRegistry()
	if _, ok := reg.Get("abc"); ok {
		t.Fatalf("empty registry returned a buffer")
	}
	b := NewBuffer([]byte("HDR"), 10*time.Second)
	reg.Set("abc", b)
	if got, ok := reg.Get("abc"); !ok || got != b {
		t.Fatalf("Get after Set = %v, %v", got, ok)
	}
	reg.Remove("abc")
	if _, ok := reg.Get("abc"); ok {
		t.Fatalf("Get after Remove still returned a buffer")
	}
}

// TestRegistryRemoveIfIsIdentitySafe pins that RemoveIf only deletes the key
// when it still holds the exact buffer passed in. This guards the live-tap
// teardown: a discarded/stale tap calling RemoveIf(key, itsOwnBuf) must never
// evict a newer buffer another tap has since registered under the same key
// (the registry-emptying double-dial race).
func TestRegistryRemoveIfIsIdentitySafe(t *testing.T) {
	reg := NewRegistry()
	older := NewBuffer([]byte("OLD"), time.Second)
	newer := NewBuffer([]byte("NEW"), time.Second)

	reg.Set("ffa", older)
	reg.Set("ffa", newer) // newer replaces older under the same key

	reg.RemoveIf("ffa", older) // stale tap tears down: must be a no-op
	if got, ok := reg.Get("ffa"); !ok || got != newer {
		t.Fatalf("RemoveIf(stale) evicted the live buffer: got %v, ok=%v", got, ok)
	}

	reg.RemoveIf("ffa", newer) // the current owner tears down: must delete
	if _, ok := reg.Get("ffa"); ok {
		t.Fatalf("RemoveIf(current) did not delete the key")
	}
}

// TestRegistryWaitNextAcrossSessions pins the per-key successive-buffer
// subscription the relay uses to span an inter-match gap: a waiter holding the
// prior session's buffer must keep waiting through the gap (cur=nil) and unblock
// when the next session registers, and Close must release it.
func TestRegistryWaitNextAcrossSessions(t *testing.T) {
	reg := NewRegistry()
	a := NewBuffer([]byte("A"), time.Second)
	b := NewBuffer([]byte("B"), time.Second)
	reg.Set("ffa", a)

	done := make(chan *Buffer, 1)
	go func() {
		next, ok := reg.WaitNext(context.Background(), "ffa", a)
		if !ok {
			done <- nil
			return
		}
		done <- next
	}()
	time.Sleep(20 * time.Millisecond)
	reg.RemoveIf("ffa", a) // inter-match gap (cur=nil): waiter must keep waiting
	time.Sleep(20 * time.Millisecond)
	reg.Set("ffa", b) // next session: waiter unblocks with b
	select {
	case got := <-done:
		if got != b {
			t.Fatalf("WaitNext returned %v, want b", got)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitNext did not return after the next session registered")
	}

	// Close releases a waiter with (nil,false) — the relay ends the response.
	closed := make(chan bool, 1)
	go func() {
		_, ok := reg.WaitNext(context.Background(), "ffa", b)
		closed <- ok
	}()
	time.Sleep(20 * time.Millisecond)
	reg.Close("ffa")
	select {
	case ok := <-closed:
		if ok {
			t.Fatal("WaitNext returned ok after Close, want closed")
		}
	case <-time.After(time.Second):
		t.Fatal("WaitNext did not return after Close")
	}
}

// TestRelayHoldsOpenAcrossSessions is the Phase 3 contract: one HTTP response
// spans two sessions, splicing the TVLe boundary + the next session's TVL1
// header inline (what the browser engine reads as an in-place map change).
func TestRelayHoldsOpenAcrossSessions(t *testing.T) {
	reg := NewRegistry()
	// delay 0 -> every appended segment is immediately releasable.
	a := NewBuffer(StreamHeader{MapName: "mapA"}.Marshal(), 0)
	a.Append(Segment{Payload: []byte("a0")})
	a.End()
	reg.Set("m1", a)

	srv := NewRelayServer(reg)
	srv.tick = 10 * time.Millisecond
	ts := httptest.NewServer(srv)
	defer ts.Close()

	bodyCh := make(chan []byte, 1)
	go func() {
		resp, err := http.Get(ts.URL + "/tv/m1")
		if err != nil {
			bodyCh <- nil
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		bodyCh <- body
	}()

	// Let the relay stream session A and park in WaitNext, then start session B.
	time.Sleep(80 * time.Millisecond)
	b := NewBuffer(StreamHeader{MapName: "mapB"}.Marshal(), 0)
	b.Append(Segment{Payload: []byte("b0")})
	b.End()
	reg.RemoveIf("m1", a)
	reg.Set("m1", b)
	time.Sleep(80 * time.Millisecond)
	reg.Close("m1") // no third session: relay ends after B

	select {
	case body := <-bodyCh:
		hA := StreamHeader{MapName: "mapA"}.Marshal()
		hB := StreamHeader{MapName: "mapB"}.Marshal()
		iA := bytes.Index(body, hA)
		iB := bytes.Index(body, hB)
		if iA < 0 || iB < 0 || iB <= iA {
			t.Fatalf("want mapA header then mapB header; iA=%d iB=%d body=%q", iA, iB, body)
		}
		if n := bytes.Count(body, []byte(endMagic)); n != 2 {
			t.Fatalf("want 2 TVLe boundaries (one per session), got %d in %q", n, body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("relay did not finish after Close")
	}
}
