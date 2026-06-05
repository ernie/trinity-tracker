// Package console holds the in-memory console-line rings the collector
// fills from engine taps (sv_conTap) and the hub serves to viewers.
// Non-persistent by design: games.log is the durable record.
package console

import "sync"

// RingSize is the scrollback depth per server.
const RingSize = 500

// subscriberBuf bounds a subscriber's channel; a viewer that falls this
// far behind is dropped (the SSE layer re-snapshots on reconnect).
const subscriberBuf = 256

// Line is one console line with a per-server monotonic sequence number,
// used to dedup scrollback against the live stream.
type Line struct {
	Seq  int64  `json:"seq"`
	Text string `json:"text"`
}

// Ring is a bounded scrollback buffer with live fan-out. One writer
// (the collector tap consumer), many subscribers.
type Ring struct {
	mu    sync.Mutex
	lines []Line // at most RingSize, oldest first
	seq   int64
	subs  map[chan Line]struct{}
	tapUp bool
}

func NewRing() *Ring {
	return &Ring{subs: make(map[chan Line]struct{})}
}

// Append adds a line, evicting the oldest past RingSize, and fans it
// out. A subscriber whose buffer is full is dropped (closed channel).
func (r *Ring) Append(text string) {
	r.mu.Lock()
	r.seq++
	l := Line{Seq: r.seq, Text: text}
	r.lines = append(r.lines, l)
	if len(r.lines) > RingSize {
		r.lines = r.lines[1:]
	}
	for ch := range r.subs {
		select {
		case ch <- l:
		default:
			delete(r.subs, ch)
			close(ch)
		}
	}
	r.mu.Unlock()
}

// Snapshot returns the current scrollback, oldest first.
func (r *Ring) Snapshot() []Line {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Line, len(r.lines))
	copy(out, r.lines)
	return out
}

// Subscribe returns the scrollback plus a channel of subsequent lines.
// The channel is closed if the subscriber falls behind. Always pair
// with Unsubscribe.
func (r *Ring) Subscribe() ([]Line, chan Line) {
	r.mu.Lock()
	defer r.mu.Unlock()
	snap := make([]Line, len(r.lines))
	copy(snap, r.lines)
	ch := make(chan Line, subscriberBuf)
	r.subs[ch] = struct{}{}
	return snap, ch
}

// Unsubscribe removes a subscriber. Safe to call after a behind-drop.
func (r *Ring) Unsubscribe(ch chan Line) {
	r.mu.Lock()
	if _, ok := r.subs[ch]; ok {
		delete(r.subs, ch)
		close(ch)
	}
	r.mu.Unlock()
}

// SetTapUp records tap connectivity (surfaced to viewers as a status).
func (r *Ring) SetTapUp(up bool) {
	r.mu.Lock()
	r.tapUp = up
	r.mu.Unlock()
}

// TapUp reports tap connectivity.
func (r *Ring) TapUp() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tapUp
}

// Registry maps server keys to rings. The collector creates rings; the
// hub (local sources) and the NATS forwarder read them.
type Registry struct {
	mu    sync.Mutex
	rings map[string]*Ring
}

func NewRegistry() *Registry {
	return &Registry{rings: make(map[string]*Ring)}
}

// Ring returns the ring for key, creating it on first use.
func (g *Registry) Ring(key string) *Ring {
	g.mu.Lock()
	defer g.mu.Unlock()
	r, ok := g.rings[key]
	if !ok {
		r = NewRing()
		g.rings[key] = r
	}
	return r
}

// Lookup returns the ring for key, or nil if none exists yet.
func (g *Registry) Lookup(key string) *Ring {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.rings[key]
}
