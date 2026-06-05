package collector

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/console"
)

// fakeTap is a scripted sv_conTap endpoint.
type fakeTap struct {
	ln   net.Listener
	port int
}

func newFakeTap(t *testing.T) *fakeTap {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	return &fakeTap{ln: ln, port: ln.Addr().(*net.TCPAddr).Port}
}

// serveOnce accepts one consumer, sends the hello + lines, then closes.
func (f *fakeTap) serveOnce(hello string, lines ...string) {
	conn, err := f.ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	conn.Write([]byte(hello))
	for _, l := range lines {
		conn.Write([]byte(l + "\n"))
	}
	time.Sleep(50 * time.Millisecond) // let the reader drain before close
}

func newTestRunner(key, gamePort string, ring *console.Ring, tapPort int, done chan struct{}) *conTapRunner {
	r := &conTapRunner{
		key:         key,
		gameAddr:    "127.0.0.1:" + gamePort,
		gamePort:    gamePort,
		ring:        ring,
		done:        done,
		backoffBase: 10 * time.Millisecond,
		statusVars: func() (map[string]string, error) {
			return map[string]string{"sv_conport": strconv.Itoa(tapPort)}, nil
		},
	}
	return r
}

func waitFor(t *testing.T, what string, pred func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if pred() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func ringTexts(r *console.Ring) []string {
	var out []string
	for _, l := range r.Snapshot() {
		out = append(out, l.Text)
	}
	return out
}

func TestConTapConsumesLinesAndMarksDisconnect(t *testing.T) {
	tap := newFakeTap(t)
	ring := console.NewRing()
	done := make(chan struct{})
	defer close(done)

	go tap.serveOnce("CON1 27970 q3dm17\n", "hello world", "^1red^7 line")
	r := newTestRunner("ffa", "27970", ring, tap.port, done)
	go r.run()

	waitFor(t, "lines + disconnect marker", func() bool {
		texts := ringTexts(ring)
		return len(texts) >= 3 && strings.Contains(texts[len(texts)-1], "connection lost")
	})
	texts := ringTexts(ring)
	if texts[0] != "hello world" || texts[1] != "^1red^7 line" {
		t.Errorf("lines = %v", texts)
	}

	// Reconnect: rediscovers, gets a fresh hello, marks reconnection.
	go tap.serveOnce("CON1 27970 q3dm1\n", "back again")
	waitFor(t, "reconnect", func() bool {
		texts := ringTexts(ring)
		return len(texts) >= 5 && texts[len(texts)-1] == "back again"
	})
	texts = ringTexts(ring)
	if !strings.Contains(texts[len(texts)-2], "reconnected") {
		t.Errorf("missing reconnect marker: %v", texts)
	}
	if !ring.TapUp() {
		t.Error("TapUp false while connected")
	}
}

func TestConTapRejectsWrongIdentity(t *testing.T) {
	tap := newFakeTap(t)
	ring := console.NewRing()
	done := make(chan struct{})
	defer close(done)

	// Hello claims a different game port: must be rejected, no lines.
	go tap.serveOnce("CON1 11111 q3dm17\n", "should not appear")
	r := newTestRunner("ffa", "27970", ring, tap.port, done)
	go r.run()

	waitFor(t, "rejection marker", func() bool { return len(ringTexts(ring)) >= 1 })
	for _, txt := range ringTexts(ring) {
		if txt == "should not appear" {
			t.Error("lines accepted despite identity mismatch")
		}
	}
	if ring.TapUp() {
		t.Error("TapUp true after rejected hello")
	}
}

func TestConTapUnavailableWhenNoPort(t *testing.T) {
	ring := console.NewRing()
	done := make(chan struct{})
	r := &conTapRunner{
		key: "ffa", gamePort: "27970", ring: ring, done: done,
		backoffBase: 10 * time.Millisecond,
		statusVars:  func() (map[string]string, error) { return map[string]string{}, nil },
	}
	go r.run()
	time.Sleep(100 * time.Millisecond)
	close(done)
	if ring.TapUp() {
		t.Error("TapUp true with no sv_conport")
	}
	if n := len(ringTexts(ring)); n != 0 {
		t.Errorf("ring has %d lines, want 0 (no noise when unavailable): %v", n, ringTexts(ring))
	}
}

func TestConTapStopsOnDone(t *testing.T) {
	tap := newFakeTap(t)
	ring := console.NewRing()
	done := make(chan struct{})

	go func() {
		conn, err := tap.ln.Accept()
		if err != nil {
			return
		}
		fmt.Fprintf(conn, "CON1 27970 q3dm17\n")
		// hold the connection open; runner must exit via done
	}()
	r := newTestRunner("ffa", "27970", ring, tap.port, done)
	exited := make(chan struct{})
	go func() { r.run(); close(exited) }()

	waitFor(t, "tap up", func() bool { return ring.TapUp() })
	close(done)
	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not exit on done")
	}
}
