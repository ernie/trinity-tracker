package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ernie/trinity-tracker/internal/console"
	"github.com/ernie/trinity-tracker/internal/natsbus"
	"github.com/nats-io/nats.go"
)

// ssePingInterval keeps idle streams alive through proxies (a quiet
// console can be silent for minutes).
const ssePingInterval = 15 * time.Second

// consoleLeaseTTL / consoleLeaseRenew drive the remote watch lease.
const (
	consoleLeaseTTL   = 30 * time.Second
	consoleLeaseRenew = 10 * time.Second
)

// handleConsoleStream serves GET /api/console/stream?source=&key= as
// SSE: a status event, the scrollback, then live lines. Same
// authorization as rcon. Local sources read the registry directly;
// remote sources go through the leased NATS relay.
func (r *Router) handleConsoleStream(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	source := req.URL.Query().Get("source")
	key := req.URL.Query().Get("key")
	if source == "" || key == "" {
		writeError(w, http.StatusBadRequest, "source and key are required")
		return
	}
	server, err := r.store.GetServerBySourceKey(req.Context(), source, key)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	role, err := r.authorizeRcon(req.Context(), server, claims)
	if err != nil {
		writeError(w, http.StatusForbidden, "you do not have console access to this server")
		return
	}

	var (
		scrollback []console.Line
		ch         chan console.Line
		tapUp      bool
		cleanup    func()
	)
	if r.localSource != "" && source == r.localSource {
		if r.consoleReg == nil {
			writeError(w, http.StatusNotImplemented, "console streaming not enabled")
			return
		}
		ring := r.consoleReg.Lookup(key)
		if ring == nil {
			writeError(w, http.StatusServiceUnavailable, "console not available for this server")
			return
		}
		scrollback, ch = ring.Subscribe()
		tapUp = ring.TapUp()
		cleanup = func() { ring.Unsubscribe(ch) }
	} else {
		if r.consoleRelay == nil {
			writeError(w, http.StatusNotImplemented, "console streaming not enabled")
			return
		}
		scrollback, ch, tapUp, cleanup, err = r.consoleRelay.subscribe(source, key, claims.Username, role)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
	}
	defer cleanup()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// The server-wide WriteTimeout (15s) would kill the stream.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	send := func(event string, payload any) bool {
		data, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		if event != "" {
			fmt.Fprintf(w, "event: %s\n", event)
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		return rc.Flush() == nil
	}

	if !send("status", map[string]bool{"tap_up": tapUp}) {
		return
	}
	var lastSeq int64
	for _, l := range scrollback {
		if !send("", l) {
			return
		}
		lastSeq = l.Seq
	}

	ping := time.NewTicker(ssePingInterval)
	defer ping.Stop()
	for {
		select {
		case <-req.Context().Done():
			return
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		case l, open := <-ch:
			if !open {
				return // dropped as slow, or relay torn down; client reconnects
			}
			// Dedup scrollback/live overlap by seq; a much-older seq
			// means the collector restarted (ring reset) — accept it.
			if l.Seq <= lastSeq && lastSeq-l.Seq < 2*console.RingSize {
				continue
			}
			lastSeq = l.Seq
			if !send("", l) {
				return
			}
		}
	}
}

// SetConsoleRegistry wires the in-process collector's console rings
// (local-source streaming).
func (r *Router) SetConsoleRegistry(reg *console.Registry) {
	r.consoleReg = reg
}

// SetConsoleClient wires the NATS console client (remote-source
// streaming via the leased relay).
func (r *Router) SetConsoleClient(c *natsbus.ConsoleClient) {
	r.consoleRelay = newConsoleRelay(c)
}

// consoleRelay multiplexes remote console streams: one NATS
// subscription + lease-renewal loop per (source, key), fanned out to
// any number of SSE viewers. Viewers issue their own Watch (fresh
// scrollback + an extra renewal); the background renewer keeps the
// lease alive between viewer arrivals.
type consoleRelay struct {
	client *natsbus.ConsoleClient

	mu      sync.Mutex
	streams map[string]*relayStream
}

type relayStream struct {
	subs    map[chan console.Line]struct{}
	stop    chan struct{}
	natsSub interface{ Unsubscribe() error }
}

func newConsoleRelay(client *natsbus.ConsoleClient) *consoleRelay {
	return &consoleRelay{client: client, streams: make(map[string]*relayStream)}
}

// subscribe attaches a viewer. Returns scrollback, the live channel,
// tap state, and a cleanup func.
func (cr *consoleRelay) subscribe(source, key, username string, role natsbus.RconRole) ([]console.Line, chan console.Line, bool, func(), error) {
	watchReq := natsbus.ConsoleWatchRequest{
		ServerKey: key,
		Username:  username,
		Role:      role,
		TTLMs:     int(consoleLeaseTTL / time.Millisecond),
	}
	reply, err := cr.client.Watch(context.Background(), source, watchReq)
	if err != nil {
		if errors.Is(err, nats.ErrNoResponders) {
			return nil, nil, false, nil, fmt.Errorf("console streaming not available from source %q (collector update required)", source)
		}
		return nil, nil, false, nil, err
	}

	id := source + "/" + key
	ch := make(chan console.Line, 256)

	cr.mu.Lock()
	st, ok := cr.streams[id]
	if !ok {
		st = &relayStream{subs: make(map[chan console.Line]struct{}), stop: make(chan struct{})}
		cr.streams[id] = st
		sub, err := cr.client.SubscribeLines(source, key, func(b natsbus.ConsoleLineBatch) {
			cr.mu.Lock()
			for _, l := range b.Lines {
				for c := range st.subs {
					select {
					case c <- l:
					default:
						delete(st.subs, c)
						close(c)
					}
				}
			}
			cr.mu.Unlock()
		})
		if err != nil {
			delete(cr.streams, id)
			cr.mu.Unlock()
			return nil, nil, false, nil, err
		}
		st.natsSub = sub
		go cr.renew(source, watchReq, st)
	}
	st.subs[ch] = struct{}{}
	cr.mu.Unlock()

	cleanup := func() {
		cr.mu.Lock()
		if _, ok := st.subs[ch]; ok {
			delete(st.subs, ch)
			close(ch)
		}
		if len(st.subs) == 0 {
			close(st.stop)
			if st.natsSub != nil {
				_ = st.natsSub.Unsubscribe()
			}
			delete(cr.streams, id)
		}
		cr.mu.Unlock()
	}
	return reply.Scrollback, ch, reply.TapUp, cleanup, nil
}

// renew keeps the collector-side lease alive while viewers remain.
// Errors are tolerated (the collector may be restarting); the lease
// simply lapses if they persist.
func (cr *consoleRelay) renew(source string, req natsbus.ConsoleWatchRequest, st *relayStream) {
	t := time.NewTicker(consoleLeaseRenew)
	defer t.Stop()
	for {
		select {
		case <-st.stop:
			return
		case <-t.C:
			_, _ = cr.client.Watch(context.Background(), source, req)
		}
	}
}
