package collector

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/ernie/trinity-tracker/internal/livestream"
)

// liveTap owns one persistent TCP connection to a co-located game server's live
// tap. The engine keeps the consumer connected across map changes, ending each
// map's session with a TVLe marker and starting the next with a fresh
// StreamHeader on the same socket. The run loop registers a new
// livestream.Buffer under the server key for each session, so viewers keep one
// URL per server across map rotations. The connection drops only when the game
// server exits or stop() is called; onClose then lets the manager clear its
// handle and redial on the next match.
type liveTap struct {
	key      string
	reg      *livestream.Registry
	conn     net.Conn
	delay    time.Duration
	firstHdr []byte // marshaled StreamHeader of the first session, read at dial
	onClose  func(*liveTap)
	stopOnce sync.Once
}

// dialLiveTap opens the TCP connection. It's separate from the header parse so
// the manager can register the in-flight conn and close it at shutdown,
// interrupting a parked read. Caller owns the conn.
func dialLiveTap(addr string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("livetap dial %s: %w", addr, err)
	}
	return conn, nil
}

// parseLiveTapHeader reads the first stream header off conn and builds the tap.
// hdrTimeout bounds this read, since the dial timeout covers only the connect.
// It's cleared afterward because reads between sessions block for minutes
// across a map change. Caller owns the tap and calls start() once the handle is
// stored.
func parseLiveTapHeader(conn net.Conn, addr, key string, reg *livestream.Registry, hdrTimeout, delay time.Duration) (*liveTap, error) {
	if hdrTimeout > 0 {
		conn.SetReadDeadline(time.Now().Add(hdrTimeout))
	}
	hdr, err := livestream.ParseStreamHeader(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("livetap header %s: %w", addr, err)
	}
	conn.SetReadDeadline(time.Time{}) // clear: steady-state reads are unbounded
	return &liveTap{key: key, reg: reg, conn: conn, delay: delay, firstHdr: hdr.Marshal()}, nil
}

// startLiveTap runs dialLiveTap and parseLiveTapHeader, failing fast if either
// fails (for example, when the server lacks sv_tvLive). Caller owns the tap and
// must start() or stop() it.
func startLiveTap(addr, key string, reg *livestream.Registry, hdrTimeout, delay time.Duration) (*liveTap, error) {
	conn, err := dialLiveTap(addr)
	if err != nil {
		return nil, err
	}
	return parseLiveTapHeader(conn, addr, key, reg, hdrTimeout, delay)
}

// defaultLiveTapHeaderTimeout bounds the first header read. A loopback engine
// with sv_tvLive on sends the header within a server frame, so this is
// generous slack.
const defaultLiveTapHeaderTimeout = 10 * time.Second

// start runs the consume loop in a goroutine. onClose runs exactly once, after
// the loop exits, so the manager can drop its handle and let the next match
// redial. Call start only after storing the tap handle.
func (t *liveTap) start(onClose func(*liveTap)) {
	t.onClose = onClose
	go t.run()
}

// run consumes successive stream sessions on the connection. Each session
// registers a fresh Buffer under the server key and is consumed until its TVLe
// marker. Between sessions the key is dropped, so the relay answers 503 with
// Retry-After during the gap. The loop ends when ParseStreamHeader fails: the
// server exited or stop() closed the connection.
func (t *liveTap) run() {
	if t.onClose != nil {
		defer t.onClose(t)
	}
	// When the tap is gone for good, close the viewer feed so relay responses
	// waiting for the next session end and browsers fall back to the recorded
	// demo. Deferred after onClose so it runs before it: a redial from onClose
	// registers a fresh buffer that must outlive this Close.
	defer t.reg.Close(t.key)
	hdrBytes := t.firstHdr
	for {
		buf := livestream.NewBuffer(hdrBytes, t.delay)
		t.reg.Set(t.key, buf)
		// ConsumeSegments returns at the session's TVLe or on a transport
		// error, and always calls buf.End() so viewers unblock.
		_ = livestream.ConsumeSegments(t.conn, buf)
		// Another path may have registered a newer buffer under the key.
		t.reg.RemoveIf(t.key, buf)

		// Block for the next session's header (the next map going live), or
		// return when the connection is gone.
		hdr, err := livestream.ParseStreamHeader(t.conn)
		if err != nil {
			return
		}
		hdrBytes = hdr.Marshal()
	}
}

// stop closes the connection, which unblocks the run loop so it deregisters its
// buffer and fires onClose. Safe to call more than once, from any goroutine,
// and before start().
func (t *liveTap) stop() {
	t.stopOnce.Do(func() {
		t.conn.Close()
	})
}
