package collector

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/config"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/livestream"
)

// serveCountingTap accepts any number of connections, counts them, and streams
// a header + one segment on each, holding the connection open. The count lets a
// test assert how many times the collector dialed.
func serveCountingTap(t *testing.T) (addr string, dials *int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	dials = new(int32)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			atomic.AddInt32(dials, 1)
			go func(c net.Conn) {
				defer c.Close()
				c.Write(livestream.StreamHeader{SvFps: 20, MaxClients: 8, MapName: "q3dm6", Timestamp: "t"}.Marshal())
				c.Write(livestream.Segment{KeyframeServerTime: 0, Payload: []byte("a")}.Marshal())
				c.Read(make([]byte, 1)) // hold open until the tap disconnects
			}(conn)
		}
	}()
	return ln.Addr().String(), dials
}

// TestStartupDoubleInitGameOpensOneStableTap pins the in-flight guard: two
// InitGames in quick succession call openLiveTap twice before the first dial
// stores its handle. The collector must dial exactly once and leave the buffer
// stably registered — not spawn a second dial whose discard path evicts the
// first dial's buffer, emptying the registry so the relay 503s for the whole match.
func TestStartupDoubleInitGameOpensOneStableTap(t *testing.T) {
	addr, dials := serveCountingTap(t)
	reg := livestream.NewRegistry()
	cfg := &config.Config{Tracker: &config.TrackerConfig{Collector: &config.CollectorConfig{LiveDelay: config.Duration(time.Second)}}}
	m := &ServerManager{cfg: cfg, liveReg: reg, servers: map[int64]*serverState{}}

	state := &serverState{
		server: domain.Server{ID: 1, Key: "ffa", Address: addr},
		match:  &domain.Match{UUID: "live-1"},
	}
	m.servers[1] = state

	// Two InitGame events at the same second → two openLiveTap calls, both under
	// the lock handleLogEvent holds, before the first dial has stored its handle.
	m.mu.Lock()
	m.openLiveTap(state)
	m.openLiveTap(state)
	m.mu.Unlock()

	waitRegistered(t, reg, "ffa", true)

	// The buffer must STAY registered — a second dial's teardown must not evict
	// it. Poll well past the dial/store window.
	for i := 0; i < 40; i++ {
		if _, ok := reg.Get("ffa"); !ok {
			t.Fatalf("registry emptied after duplicate openLiveTap (the 503 race)")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt32(dials); got != 1 {
		t.Fatalf("expected exactly 1 dial for the double-InitGame, got %d", got)
	}
}

// TestStopWithDialInFlightDoesNotRegister is the regression test for the
// dial-goroutine leak: openLiveTap's dial runs in a goroutine that was NOT
// tracked by m.wg, and Stop() never deletes the server, so a dial that
// completes after Stop() found the server still present, stored its handle, and
// registered a Buffer + consume goroutine AFTER "shutdown complete." Stop() must
// instead await the dial (m.wg) and the dial must bail without registering once
// m.done is closed.
func TestStopWithDialInFlightDoesNotRegister(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	release := make(chan struct{}) // test → server: send the header now
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		<-release // park the dial in ParseStreamHeader until released
		conn.Write(livestream.StreamHeader{SvFps: 20, MaxClients: 8, MapName: "q3dm6", Timestamp: "t"}.Marshal())
		conn.Read(make([]byte, 1)) // hold open
	}()

	reg := livestream.NewRegistry()
	cfg := &config.Config{Tracker: &config.TrackerConfig{Collector: &config.CollectorConfig{LiveDelay: config.Duration(time.Second)}}}
	state := &serverState{
		server: domain.Server{ID: 1, Key: "ffa", Address: ln.Addr().String()},
		match:  &domain.Match{UUID: "live-1"},
	}
	m := &ServerManager{cfg: cfg, liveReg: reg, servers: map[int64]*serverState{1: state}, done: make(chan struct{})}

	// Dial starts and parks in ParseStreamHeader (no header sent yet).
	m.mu.Lock()
	m.openLiveTap(state)
	m.mu.Unlock()

	// Shut down while the dial is in flight. After the fix Stop() waits on the
	// dial goroutine (m.wg); run it concurrently and release the header so the
	// parked parse completes and the goroutine can observe m.done closed.
	stopped := make(chan struct{})
	go func() { m.Stop(); close(stopped) }()
	time.Sleep(50 * time.Millisecond) // let Stop() close m.done
	close(release)                    // dial unblocks; must bail without registering

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return — dial goroutine not awaited or never unblocked")
	}

	// A dial that finishes after Stop() must NOT register a buffer or spawn a
	// consumer. Poll past the window the late dial would otherwise need.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, ok := reg.Get("ffa"); ok {
			t.Fatal("in-flight dial registered a buffer past Stop() (leaked goroutine/socket/registry entry)")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestLiveTapRedialsAfterMidMatchDrop pins the persistent-tap goal across a
// transient loopback drop: if the tap's connection drops mid-match (e.g. the
// engine dropped a slow consumer) while the match is still live, the collector
// must redial rather than leave live black (relay 503) for the rest of the
// match. openLiveTap only fires from InitGame, so without a redial in
// onLiveTapClosed a mid-match drop is unrecoverable.
func TestLiveTapRedialsAfterMidMatchDrop(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	var conns int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			first := atomic.AddInt32(&conns, 1) == 1
			go func(c net.Conn, first bool) {
				defer c.Close()
				c.Write(livestream.StreamHeader{SvFps: 20, MaxClients: 8, MapName: "q3dm6", Timestamp: "t"}.Marshal())
				c.Write(livestream.Segment{KeyframeServerTime: 0, Payload: []byte("a")}.Marshal())
				if first {
					time.Sleep(20 * time.Millisecond)
					return // transient mid-match drop: close the first connection
				}
				c.Read(make([]byte, 1)) // hold the redial open
			}(conn, first)
		}
	}()

	reg := livestream.NewRegistry()
	cfg := &config.Config{Tracker: &config.TrackerConfig{Collector: &config.CollectorConfig{LiveDelay: config.Duration(time.Second)}}}
	state := &serverState{
		server: domain.Server{ID: 1, Key: "ffa", Address: ln.Addr().String()},
		match:  &domain.Match{UUID: "live-1"},
	}
	m := &ServerManager{cfg: cfg, liveReg: reg, servers: map[int64]*serverState{1: state}, done: make(chan struct{})}

	m.mu.Lock()
	m.openLiveTap(state)
	m.mu.Unlock()
	waitRegistered(t, reg, "ffa", true) // initial tap

	// The first connection drops; with the match still active the tap must
	// redial — observed as a second dial.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&conns) < 2 {
		time.Sleep(5 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&conns); got < 2 {
		t.Fatalf("tap did not redial after a mid-match drop (dials=%d)", got)
	}
	waitRegistered(t, reg, "ffa", true) // re-registered by the redial

	m.mu.Lock()
	m.closeLiveTap(state)
	m.mu.Unlock()
}

// TestStopClosesParkedDialPromptly pins that Stop() closes an in-flight dial's
// conn so a goroutine parked in the header parse unblocks immediately, rather
// than waiting out the (long) read deadline. The deadline stays as a fail-safe;
// shutdown shouldn't depend on it.
func TestStopClosesParkedDialPromptly(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	accepted := make(chan struct{}, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		accepted <- struct{}{}
		defer conn.Close()
		conn.Read(make([]byte, 1)) // never send a header; hold until closed
	}()

	reg := livestream.NewRegistry()
	cfg := &config.Config{Tracker: &config.TrackerConfig{Collector: &config.CollectorConfig{LiveDelay: config.Duration(time.Second)}}}
	state := &serverState{
		server: domain.Server{ID: 1, Key: "ffa", Address: ln.Addr().String()},
		match:  &domain.Match{UUID: "live-1"},
	}
	m := &ServerManager{
		cfg:               cfg,
		liveReg:           reg,
		servers:           map[int64]*serverState{1: state},
		done:              make(chan struct{}),
		liveTapHdrTimeout: 30 * time.Second, // long: if Stop waited this out, the test would hang
	}

	m.mu.Lock()
	m.openLiveTap(state)
	m.mu.Unlock()

	select {
	case <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("dial never connected")
	}
	time.Sleep(50 * time.Millisecond) // let the goroutine register the conn + enter the parse

	start := time.Now()
	m.Stop()
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Stop() took %v with a parked dial — in-flight conn not closed (waited out the deadline)", elapsed)
	}
}

// TestLiveTapRedialPersistsBeyondFourAttempts pins that a mid-match redial keeps
// retrying for the life of the match instead of giving up after a fixed count,
// so a same-match outage longer than a few seconds (a loopback hiccup, an
// overloaded box, a maintenance pause that doesn't restart the engine) still
// recovers. The server accepts then immediately drops each conn (header parse
// fails), forcing repeated redials; we assert it tries well past the old
// 4-attempt limit.
func TestLiveTapRedialPersistsBeyondFourAttempts(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	var accepts int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			atomic.AddInt32(&accepts, 1)
			c.Close() // drop immediately → parse fails → redial retries
		}
	}()

	reg := livestream.NewRegistry()
	cfg := &config.Config{Tracker: &config.TrackerConfig{Collector: &config.CollectorConfig{LiveDelay: config.Duration(time.Second)}}}
	state := &serverState{
		server: domain.Server{ID: 1, Key: "ffa", Address: ln.Addr().String()},
		match:  &domain.Match{UUID: "live-1"},
	}
	m := &ServerManager{
		cfg:               cfg,
		liveReg:           reg,
		servers:           map[int64]*serverState{1: state},
		done:              make(chan struct{}),
		liveTapRedialBase: 5 * time.Millisecond, // fast attempts for the test
	}

	// Drive the redial loop directly (as onLiveTapClosed would on a drop).
	m.wg.Add(1)
	go m.redialLiveTap(state)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&accepts) < 6 {
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&accepts); got < 6 {
		t.Fatalf("redial made only %d attempts; the old fixed 4-attempt schedule would have given up (expected persistence)", got)
	}

	close(m.done) // stop the loop
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("redial goroutine did not stop after m.done closed")
	}
}

// TestStartupTapsInProgressMatch pins that when the collector starts (or
// restarts) and the replayed log shows a match already in progress, it taps that
// match immediately rather than waiting for the next InitGame. This is what lets
// a `trinity` restart re-tap a live game server mid-match.
func TestStartupTapsInProgressMatch(t *testing.T) {
	addr := serveFakeTap(t) // the still-running game server's TV listener

	// A log whose last match-relevant line is an InitGame with no following
	// Shutdown — i.e. a match in progress when the collector (re)starts.
	dir := t.TempDir()
	logPath := filepath.Join(dir, "ffa.log")
	line := "2026-06-02T12:00:00 InitGame: \\mapname\\q3dm1\\g_gametype\\0\\g_matchUUID\\inprogress-uuid\n"
	if err := os.WriteFile(logPath, []byte(line), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	reg := livestream.NewRegistry()
	cfg := &config.Config{Tracker: &config.TrackerConfig{Collector: &config.CollectorConfig{LiveDelay: config.Duration(time.Second)}}}
	state := &serverState{
		server:        domain.Server{ID: 1, Key: "ffa", Address: addr},
		clients:       make(map[int]*clientState),
		trinityNonces: make(map[int]string),
		openSessions:  make(map[string]bool),
	}
	m := &ServerManager{
		cfg:     cfg,
		liveReg: reg,
		servers: map[int64]*serverState{1: state},
		tailers: make(map[int64]*LogTailer),
		done:    make(chan struct{}),
	}
	defer m.Stop()

	// attachTailer replays the log (sets state.match) then taps the in-progress match.
	if !m.attachTailer(context.Background(), "ffa", logPath, 1, time.Time{}) {
		t.Fatal("attachTailer returned false")
	}

	waitRegistered(t, reg, "ffa", true) // tapped mid-match, no new InitGame needed
}

func TestSetLiveRegistry(t *testing.T) {
	m := &ServerManager{}
	reg := livestream.NewRegistry()
	m.SetLiveRegistry(reg)
	if m.liveReg != reg {
		t.Fatalf("SetLiveRegistry did not store the registry")
	}
}

func TestRosterReportsIsLive(t *testing.T) {
	cfg := &config.Config{Tracker: &config.TrackerConfig{Collector: &config.CollectorConfig{}}}
	state := &serverState{server: domain.Server{ID: 1, Key: "ffa", Address: "127.0.0.1:27960"}}
	m := &ServerManager{cfg: cfg, servers: map[int64]*serverState{1: state}}

	for _, rs := range m.Roster() {
		if rs.Key == "ffa" && rs.IsLive {
			t.Fatal("IsLive true before any tap opened")
		}
	}

	state.liveTap = &liveTap{key: "ffa"}
	found := false
	for _, rs := range m.Roster() {
		if rs.Key == "ffa" {
			found = true
			if !rs.IsLive {
				t.Fatal("IsLive false with an open tap")
			}
		}
	}
	if !found {
		t.Fatal("server ffa missing from roster")
	}
}

func TestOpenCloseLiveTapLifecycle(t *testing.T) {
	addr := serveFakeTap(t)
	reg := livestream.NewRegistry()
	cfg := &config.Config{Tracker: &config.TrackerConfig{Collector: &config.CollectorConfig{LiveDelay: config.Duration(time.Second)}}}
	state := &serverState{
		server: domain.Server{ID: 1, Key: "ffa", Address: addr},
		match:  &domain.Match{UUID: "live-1"},
	}
	m := &ServerManager{cfg: cfg, liveReg: reg, servers: map[int64]*serverState{1: state}}

	// openLiveTap/closeLiveTap are only ever called by handleLogEvent while
	// it holds m.mu; the dial goroutine re-acquires m.mu to store the handle.
	// Drive the helpers under the same lock so the test respects that contract.
	m.mu.Lock()
	m.openLiveTap(state)
	m.mu.Unlock()
	waitRegistered(t, reg, "ffa", true)

	m.mu.Lock()
	m.closeLiveTap(state)
	m.mu.Unlock()
	waitRegistered(t, reg, "ffa", false) // run loop deregisters when the conn closes
}

func TestLiveTapDialDiscardedWhenServerRemoved(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	release := make(chan struct{}) // test → server: ok to send the header now
	done := make(chan struct{})    // server → test: client disconnected (tap stopped)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		<-release // block in-flight: startLiveTap is parked in ParseStreamHeader
		conn.Write(livestream.StreamHeader{SvFps: 20, MaxClients: 8, MapName: "q3dm6", Timestamp: "t"}.Marshal())
		// Block until the client (the tap) closes the connection, which is what
		// tap.stop() does — this is our signal the goroutine reached stop().
		_, _ = conn.Read(make([]byte, 1))
		close(done)
	}()

	reg := livestream.NewRegistry()
	cfg := &config.Config{Tracker: &config.TrackerConfig{Collector: &config.CollectorConfig{LiveDelay: config.Duration(time.Second)}}}
	state := &serverState{
		server: domain.Server{ID: 1, Key: "ffa", Address: ln.Addr().String()},
		match:  &domain.Match{UUID: "live-x"},
	}
	m := &ServerManager{cfg: cfg, liveReg: reg, servers: map[int64]*serverState{1: state}}

	// Open the tap; the dial goroutine connects but parks in ParseStreamHeader
	// (the fake hasn't sent the header). Mirror production locking.
	m.mu.Lock()
	m.openLiveTap(state)
	m.mu.Unlock()

	// The server is removed (reconfigured) while the dial is still in flight.
	m.mu.Lock()
	delete(m.servers, 1)
	m.mu.Unlock()

	// Now let the dial complete: startLiveTap returns, the goroutine re-acquires
	// m.mu, finds the server gone, and stops the late tap before starting it.
	close(release)

	select {
	case <-done:
		// tap.stop() closed the conn.
	case <-time.After(2 * time.Second):
		t.Fatal("dial goroutine never stopped the late tap (conn stayed open) — leaked tap")
	}

	// The discarded tap was never started, so it never registered a buffer.
	// Assert nothing is (or becomes) registered.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, ok := reg.Get("ffa"); ok {
			t.Fatal("a discarded dial registered a buffer (registry leak)")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
