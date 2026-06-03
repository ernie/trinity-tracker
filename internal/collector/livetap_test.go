package collector

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/livestream"
)

// liveHeader streams the registry's buffer for key just long enough to read
// the StreamHeader it is currently serving, so a test can assert WHICH map's
// session is live. Buffers built with delay 0 release immediately.
func liveHeader(t *testing.T, reg *livestream.Registry, key string) (livestream.StreamHeader, bool) {
	t.Helper()
	b, ok := reg.Get(key)
	if !ok {
		return livestream.StreamHeader{}, false
	}
	var out bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = b.Stream(ctx, &out, func() {}, 5*time.Millisecond); close(done) }()
	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done
	if out.Len() < 4 {
		return livestream.StreamHeader{}, false
	}
	h, err := livestream.ParseStreamHeader(bytes.NewReader(out.Bytes()))
	if err != nil {
		return livestream.StreamHeader{}, false
	}
	return h, true
}

func waitForLiveMap(t *testing.T, reg *livestream.Registry, key, mapName string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if h, ok := liveHeader(t, reg, key); ok && h.MapName == mapName {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for live map %q under key %q", mapName, key)
}

// serveReKeyTap emits two stream sessions on ONE connection — map A, then
// (after the test releases gateB) a TVLe and map B — mirroring how the engine
// keeps a consumer connected across a map change and re-sends a header. It
// holds the connection open after map B so its buffer stays "live".
func serveReKeyTap(t *testing.T, mapA, mapB string) (addr string, gateB chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	gateB = make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.Write(livestream.StreamHeader{SvFps: 20, MaxClients: 8, MapName: mapA, Timestamp: "t"}.Marshal())
		conn.Write(livestream.Segment{KeyframeServerTime: 0, Payload: []byte("a")}.Marshal())
		<-gateB // hold session A live until the test has observed it
		conn.Write([]byte("TVLe"))
		conn.Write(livestream.StreamHeader{SvFps: 20, MaxClients: 8, MapName: mapB, Timestamp: "t"}.Marshal())
		conn.Write(livestream.Segment{KeyframeServerTime: 0, Payload: []byte("b")}.Marshal())
		select {} // keep session B's connection open
	}()
	return ln.Addr().String(), gateB
}

// TestLiveTapReKeysOnNewStreamHeader pins the persistent-tap contract: when the
// engine ends a map's session (TVLe) and starts the next with a fresh header on
// the SAME connection, the tap re-registers a new buffer under the same key, so
// viewers follow the map change without the collector reconnecting. This is the
// Layer-2 fix — without it ConsumeSegments treats the first TVLe as terminal and
// the next map never streams.
func TestLiveTapReKeysOnNewStreamHeader(t *testing.T) {
	addr, gateB := serveReKeyTap(t, "q3dm6", "q3dm1")
	reg := livestream.NewRegistry()

	tap, err := startLiveTap(addr, "ffa", reg, 5*time.Second, 0)
	if err != nil {
		t.Fatalf("startLiveTap: %v", err)
	}
	tap.start(func(*liveTap) {})
	defer tap.stop()

	waitForLiveMap(t, reg, "ffa", "q3dm6") // session A
	close(gateB)
	waitForLiveMap(t, reg, "ffa", "q3dm1") // re-keyed to session B
}

// TestLiveTapRemovesBufferAndNotifiesOnConnClose pins that when the tap's
// connection drops (the game server exited), the buffer is dropped from the
// registry and onClose fires so the manager can clear its handle and redial.
func TestLiveTapRemovesBufferAndNotifiesOnConnClose(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Write(livestream.StreamHeader{SvFps: 20, MaxClients: 8, MapName: "q3dm6", Timestamp: "t"}.Marshal())
		conn.Write(livestream.Segment{KeyframeServerTime: 0, Payload: []byte("a")}.Marshal())
		conn.Close() // server exits mid-stream
	}()

	reg := livestream.NewRegistry()
	tap, err := startLiveTap(ln.Addr().String(), "ffa", reg, 5*time.Second, 0)
	if err != nil {
		t.Fatalf("startLiveTap: %v", err)
	}
	closed := make(chan struct{})
	tap.start(func(*liveTap) { close(closed) })

	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("onClose never fired after the connection dropped")
	}
	if _, ok := reg.Get("ffa"); ok {
		t.Fatal("buffer still registered after the connection dropped")
	}
}

// serveFakeTap listens on 127.0.0.1:0 and, on connect, writes a header + two
// segments, then keeps the connection open (an ongoing live match) until the
// client (the tap) disconnects. Returns the dial address. Because no TVLe is
// sent, the tap's buffer stays registered for the life of the connection.
func serveFakeTap(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.Write(livestream.StreamHeader{SvFps: 20, MaxClients: 8, MapName: "q3dm6", Timestamp: "t"}.Marshal())
		conn.Write(livestream.Segment{KeyframeServerTime: 0, Payload: []byte("seg0")}.Marshal())
		conn.Write(livestream.Segment{KeyframeServerTime: 2000, Payload: []byte("seg1")}.Marshal())
		// Hold the session open; return (closing the conn) once the tap
		// disconnects, so the goroutine doesn't leak past the test.
		_, _ = conn.Read(make([]byte, 1))
	}()
	return ln.Addr().String()
}

func waitRegistered(t *testing.T, reg *livestream.Registry, key string, want bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := reg.Get(key); ok == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for reg.Get(%q) registered=%v", key, want)
}

func TestLiveTapRegistersAndConsumes(t *testing.T) {
	addr := serveFakeTap(t)
	reg := livestream.NewRegistry()

	tap, err := startLiveTap(addr, "ffa", reg, 5*time.Second, 10*time.Second)
	if err != nil {
		t.Fatalf("startLiveTap: %v", err)
	}
	tap.start(func(*liveTap) {})
	defer tap.stop()

	// Once started, the run loop registers a buffer under the server key.
	waitRegistered(t, reg, "ffa", true)
}

// TestStartLiveTapHeaderReadDeadline pins that a connected-but-silent engine
// can't park the dial in ParseStreamHeader forever: the read deadline turns it
// into a bounded error instead of an indefinite block.
func TestStartLiveTapHeaderReadDeadline(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.Read(make([]byte, 1)) // accept, send nothing, hold the conn open
	}()

	reg := livestream.NewRegistry()
	start := time.Now()
	if _, err := startLiveTap(ln.Addr().String(), "ffa", reg, 150*time.Millisecond, time.Second); err == nil {
		t.Fatal("expected a header read timeout against a silent server, got nil error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("read deadline not honored: header parse blocked for %v", elapsed)
	}
}

func TestLiveTapStopRemovesFromRegistry(t *testing.T) {
	addr := serveFakeTap(t)
	reg := livestream.NewRegistry()
	tap, err := startLiveTap(addr, "ffa", reg, 5*time.Second, time.Second)
	if err != nil {
		t.Fatalf("startLiveTap: %v", err)
	}
	tap.start(func(*liveTap) {})
	waitRegistered(t, reg, "ffa", true)
	tap.stop()
	waitRegistered(t, reg, "ffa", false)
	tap.stop() // idempotent
}
