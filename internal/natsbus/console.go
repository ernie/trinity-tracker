package natsbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ernie/trinity-tracker/internal/console"
	"github.com/nats-io/nats.go"
)

// Console following runs hub → collector with the same two-check trust
// model as RCON: the hub authorizes the viewer, the collector
// re-validates the role against its own per-server config. Lines cross
// the WAN only while a TTL lease is live — expiry is the off-switch,
// so a vanished hub can't leak a forever-stream.
const (
	ConsoleWatchSubjectPrefix = "trinity.console.watch."
	ConsoleLinesSubjectPrefix = "trinity.console.lines."

	defaultConsoleWatchTimeout = 5 * time.Second
	// consoleFlushInterval batches line publishes (console output is
	// bursty; per-line WAN messages would be wasteful).
	consoleFlushInterval = 100 * time.Millisecond
)

// ConsoleWatchRequest asks the collector to (re)lease the line stream
// for one server. Role semantics match RconExecRequest.
type ConsoleWatchRequest struct {
	ServerKey string   `json:"server_key"`
	Username  string   `json:"username"`
	Role      RconRole `json:"role"`
	TTLMs     int      `json:"ttl_ms"`
}

// ConsoleWatchReply carries scrollback so a fresh viewer paints
// instantly; live lines follow on the lines subject (dedup by Seq).
type ConsoleWatchReply struct {
	Error      string         `json:"error,omitempty"`
	TapUp      bool           `json:"tap_up"`
	Scrollback []console.Line `json:"scrollback,omitempty"`
}

// ConsoleLineBatch is the lines-subject payload.
type ConsoleLineBatch struct {
	Lines []console.Line `json:"lines"`
}

// ConsoleAuthorizer is the collector-side re-validation hook (the
// rcon_proxy pattern): may this role view this server's console?
type ConsoleAuthorizer interface {
	AuthorizeConsole(serverKey string, role RconRole) error
}

// ConsoleForwarder owns the collector side: lease bookkeeping and the
// per-server pump goroutines that publish ring lines while leased.
type ConsoleForwarder struct {
	nc     *nats.Conn
	source string
	reg    *console.Registry
	auth   ConsoleAuthorizer

	mu     sync.Mutex
	sub    *nats.Subscription
	leases map[string]*consoleLease
}

type consoleLease struct {
	expiry time.Time
	stop   chan struct{} // closed when the pump exits
}

// RegisterConsoleForwarder subscribes the collector to its watch
// subject and serves leases from the given registry.
func RegisterConsoleForwarder(nc *nats.Conn, source string, reg *console.Registry, auth ConsoleAuthorizer) (*ConsoleForwarder, error) {
	if nc == nil || reg == nil || auth == nil {
		return nil, fmt.Errorf("natsbus.RegisterConsoleForwarder: nc, registry, and authorizer are required")
	}
	if source == "" {
		return nil, fmt.Errorf("natsbus.RegisterConsoleForwarder: source is required")
	}
	f := &ConsoleForwarder{nc: nc, source: source, reg: reg, auth: auth, leases: make(map[string]*consoleLease)}
	sub, err := nc.Subscribe(ConsoleWatchSubjectPrefix+source, f.handleWatch)
	if err != nil {
		return nil, fmt.Errorf("natsbus: subscribe console watch: %w", err)
	}
	if err := nc.Flush(); err != nil {
		_ = sub.Unsubscribe()
		return nil, fmt.Errorf("natsbus: flush console watch subscription: %w", err)
	}
	f.sub = sub
	log.Printf("natsbus: collector subscribed to %s%s", ConsoleWatchSubjectPrefix, source)
	return f, nil
}

func (f *ConsoleForwarder) Stop() {
	if f == nil {
		return
	}
	f.mu.Lock()
	if f.sub != nil {
		_ = f.sub.Unsubscribe()
		f.sub = nil
	}
	for _, l := range f.leases {
		l.expiry = time.Time{} // pumps notice on their next tick
	}
	f.mu.Unlock()
}

func (f *ConsoleForwarder) handleWatch(m *nats.Msg) {
	var req ConsoleWatchRequest
	if err := json.Unmarshal(m.Data, &req); err != nil {
		respond(m, ConsoleWatchReply{Error: "invalid request"})
		return
	}
	if err := f.auth.AuthorizeConsole(req.ServerKey, req.Role); err != nil {
		respond(m, ConsoleWatchReply{Error: err.Error()})
		return
	}
	ttl := time.Duration(req.TTLMs) * time.Millisecond
	if ttl <= 0 || ttl > time.Minute {
		ttl = 30 * time.Second
	}
	ring := f.reg.Lookup(req.ServerKey)
	if ring == nil {
		respond(m, ConsoleWatchReply{Error: "unknown server key"})
		return
	}

	f.mu.Lock()
	l, ok := f.leases[req.ServerKey]
	if ok {
		select {
		case <-l.stop: // pump already exited; start a fresh one
			ok = false
		default:
		}
	}
	var scrollback []console.Line
	if !ok {
		l = &consoleLease{expiry: time.Now().Add(ttl), stop: make(chan struct{})}
		f.leases[req.ServerKey] = l
		// Subscribe before snapshotting so no line can fall between
		// the reply's scrollback and the pump's stream (overlap is
		// deduped hub-side by seq).
		snap, ch := ring.Subscribe()
		scrollback = snap
		go f.pump(req.ServerKey, ring, l, ch)
	} else {
		l.expiry = time.Now().Add(ttl)
		scrollback = ring.Snapshot()
	}
	f.mu.Unlock()

	respond(m, ConsoleWatchReply{TapUp: ring.TapUp(), Scrollback: scrollback})
}

// pump publishes ring lines in batches while the lease is live. ch is
// the subscription handleWatch opened before snapshotting.
func (f *ConsoleForwarder) pump(key string, ring *console.Ring, l *consoleLease, ch chan console.Line) {
	defer close(l.stop)
	subject := ConsoleLinesSubjectPrefix + f.source + "." + key
	defer func() { ring.Unsubscribe(ch) }()

	ticker := time.NewTicker(consoleFlushInterval)
	defer ticker.Stop()
	var batch []console.Line

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if data, err := json.Marshal(ConsoleLineBatch{Lines: batch}); err == nil {
			_ = f.nc.Publish(subject, data)
		}
		batch = nil
	}

	for {
		select {
		case line, open := <-ch:
			if !open { // dropped as a slow subscriber; resubscribe
				flush()
				_, ch = ring.Subscribe()
				continue
			}
			batch = append(batch, line)
		case <-ticker.C:
			flush()
			f.mu.Lock()
			cur, ok := f.leases[key]
			if !ok || cur != l { // replaced — a newer pump owns the key
				f.mu.Unlock()
				return
			}
			expired := time.Now().After(cur.expiry)
			if expired {
				delete(f.leases, key)
			}
			f.mu.Unlock()
			if expired {
				return
			}
		}
	}
}

// ConsoleClient is the hub-side watch issuer.
type ConsoleClient struct {
	nc *nats.Conn
}

func NewConsoleClient(nc *nats.Conn) (*ConsoleClient, error) {
	if nc == nil {
		return nil, fmt.Errorf("natsbus.NewConsoleClient: NATS connection is required")
	}
	return &ConsoleClient{nc: nc}, nil
}

// Watch issues (or renews) a lease and returns scrollback + tap state.
func (c *ConsoleClient) Watch(ctx context.Context, source string, req ConsoleWatchRequest) (*ConsoleWatchReply, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("natsbus.ConsoleClient.Watch: marshal: %w", err)
	}
	reqCtx, cancel := context.WithTimeout(ctx, defaultConsoleWatchTimeout)
	defer cancel()
	msg, err := c.nc.RequestWithContext(reqCtx, ConsoleWatchSubjectPrefix+source, body)
	if err != nil {
		return nil, fmt.Errorf("natsbus.ConsoleClient.Watch: %s: %w", source, err)
	}
	var reply ConsoleWatchReply
	if err := json.Unmarshal(msg.Data, &reply); err != nil {
		return nil, fmt.Errorf("natsbus.ConsoleClient.Watch: unmarshal: %w", err)
	}
	if reply.Error != "" {
		return nil, fmt.Errorf("console watch: %s", reply.Error)
	}
	return &reply, nil
}

// SubscribeLines delivers line batches for one (source, key).
func (c *ConsoleClient) SubscribeLines(source, key string, fn func(ConsoleLineBatch)) (*nats.Subscription, error) {
	return c.nc.Subscribe(ConsoleLinesSubjectPrefix+source+"."+key, func(m *nats.Msg) {
		var batch ConsoleLineBatch
		if err := json.Unmarshal(m.Data, &batch); err != nil {
			return
		}
		fn(batch)
	})
}
