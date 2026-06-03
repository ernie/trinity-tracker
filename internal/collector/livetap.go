package collector

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/ernie/trinity-tracker/internal/livestream"
)

// liveTap owns one persistent TCP connection to a co-located game server's live
// tap. The engine keeps a consumer connected ACROSS map changes — ending each
// map's session with a TVLe marker and starting the next with a fresh
// StreamHeader on the same socket — so the tap outlives a single match. Its run
// loop re-keys a new livestream.Buffer under the server key for each stream
// session, giving viewers a stable per-server URL across map rotations without
// the collector reconnecting. The connection is dropped only when the game
// server itself exits (or stop() is called), at which point onClose fires so
// the manager can clear its handle and redial on the next match.
type liveTap struct {
	key      string
	reg      *livestream.Registry
	conn     net.Conn
	delay    time.Duration
	firstHdr []byte // marshaled StreamHeader of the first session, read at dial
	onClose  func(*liveTap)
	stopOnce sync.Once
}

// dialLiveTap opens the TCP connection (2s timeout). Split from the header parse
// so the manager can register the in-flight conn and close it on shutdown,
// interrupting a parked read. Caller owns the conn.
func dialLiveTap(addr string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("livetap dial %s: %w", addr, err)
	}
	return conn, nil
}

// parseLiveTapHeader reads the first stream header off conn and builds the tap.
// hdrTimeout bounds this read so a connected-but-silent engine can't park the
// goroutine (the dial timeout covers connect, not the read). It's cleared once
// the header is read because the run loop's inter-session reads legitimately
// block for minutes across a map change. Caller owns the tap; call start() once
// the handle is stored.
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

// startLiveTap composes dialLiveTap + parseLiveTapHeader, failing fast if either
// does (e.g. the server lacks sv_tvLive). Caller owns the tap; must start() or stop() it.
func startLiveTap(addr, key string, reg *livestream.Registry, hdrTimeout, delay time.Duration) (*liveTap, error) {
	conn, err := dialLiveTap(addr)
	if err != nil {
		return nil, err
	}
	return parseLiveTapHeader(conn, addr, key, reg, hdrTimeout, delay)
}

// defaultLiveTapHeaderTimeout bounds the first header read at dial. A loopback
// engine with sv_tvLive on sends the header within a server frame (~50 ms);
// this is generous slack before declaring the tap unavailable.
const defaultLiveTapHeaderTimeout = 10 * time.Second

// start launches the consume loop in a goroutine. onClose is invoked exactly
// once, after the loop exits (the connection dropped or stop() was called), so
// the manager can drop its handle and let the next match redial. Call start
// only after storing the tap handle.
func (t *liveTap) start(onClose func(*liveTap)) {
	t.onClose = onClose
	go t.run()
}

// run consumes successive stream sessions on the persistent connection. Each
// session registers a fresh Buffer under the server key (re-keying viewers onto
// the new map) and is consumed until its TVLe marker; between sessions the key
// is dropped so the relay reports 503/Retry-After during the inter-match gap.
// The loop ends when ParseStreamHeader fails — the server closed the connection
// (it exited) or stop() did.
func (t *liveTap) run() {
	if t.onClose != nil {
		defer t.onClose(t)
	}
	hdrBytes := t.firstHdr
	for {
		buf := livestream.NewBuffer(hdrBytes, t.delay)
		t.reg.Set(t.key, buf)
		// ConsumeSegments returns on the session's TVLe (clean) or a transport
		// error; it always calls buf.End() so viewers unblock either way.
		_ = livestream.ConsumeSegments(t.conn, buf)
		// Identity-safe: only drop the key if it still holds THIS session's
		// buffer, never a newer one another path may have registered.
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

// stop closes the connection, which unblocks the run loop (ParseStreamHeader or
// ConsumeSegments returns) and drives it to deregister its buffer and fire
// onClose. Safe to call more than once and from multiple goroutines, including
// before start() (in which case nothing was ever registered).
func (t *liveTap) stop() {
	t.stopOnce.Do(func() {
		t.conn.Close()
	})
}
