package collector

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ernie/trinity-tracker/internal/config"
	"github.com/ernie/trinity-tracker/internal/console"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/hub"
	"github.com/ernie/trinity-tracker/internal/livestream"
)

// ServerManager orchestrates log parsing for every configured game server.
type ServerManager struct {
	cfg     *config.Config
	server  hub.ServerClient
	rpc     hub.RPCClient
	pub     hub.FactPublisher
	livePub hub.LiveEventPublisher
	liveReg *livestream.Registry
	// consoleReg receives console-tap lines per server. See SetConsoleRegistry.
	consoleReg *console.Registry
	q3client   *Q3Client

	kickRegistrar func() // wired by SetRegistrarKick; nil until then

	events chan domain.Event

	// replayCutoffs holds each server's replay/live boundary, loaded from
	// the publisher's Watermark.PerServer at startup, so a tailer compares
	// its log against the last event published for that server. Servers
	// without an entry fall back to LastMatchEndedAt (see cutoffFor).
	replayCutoffs map[int64]time.Time

	mu              sync.RWMutex
	servers         map[int64]*serverState
	tailers         map[int64]*LogTailer
	done            chan struct{}
	wg              sync.WaitGroup
	startupComplete bool

	// liveTapHdrTimeout bounds the first-header read of a live-tap dial.
	// Zero means defaultLiveTapHeaderTimeout; tests shrink it.
	liveTapHdrTimeout time.Duration

	// liveTapRedialBase overrides the first mid-match redial backoff.
	// Zero means defaultLiveTapRedialBase; tests shrink it.
	liveTapRedialBase time.Duration

	// inflightDials holds connected live-tap conns not yet stored (parked in
	// the header parse). Stop() closes them so a parked read unblocks at once.
	// Guarded by mu.
	inflightDials map[net.Conn]struct{}
}

type serverState struct {
	server domain.Server
	match  *domain.Match
	// handshakeRequired tracks g_trinityHandshake for the current match.
	// When true, greeting waits for the TrinityHandshake event so its auth
	// proof can apply to the welcome.
	handshakeRequired bool
	clients           map[int]*clientState    // client ID -> client state
	previousClients   map[string]*clientState // GUID -> accumulated stats from previous stints
	lastInitGame      time.Time               // dedupe InitGame and skip fake ShutdownGame at same timestamp
	matchState        string                  // "waiting", "warmup", "active", "overtime", "intermission"
	matchStarted      bool                    // true once MatchStart has been published (at WarmupEnd)
	matchFlushed      bool                    // true once match stats have been flushed
	warmupDuration    int                     // warmup duration in seconds (set when warmup starts)
	pendingExit       *string                 // exit reason from Exit event (deferred until scores captured)
	pendingExitAt     time.Time               // timestamp of Exit event
	pendingRedScore   *int                    // team scores captured at Exit time (before server resets)
	pendingBlueScore  *int

	// GUIDs the collector knows have an open session on this server.
	// Mirrors the hub's sessions table for live, on-server players;
	// persists across InitGame so a map-change ClientBegin can be told
	// apart from a fresh connect (the game re-emits userinfo and Begin for
	// every connected client at each map change). When set, the Begin is a
	// continuation: emit FactPresenceSnapshot and skip the greeting. When
	// unset, it's a join: emit FactPlayerJoin and greet.
	openSessions map[string]bool

	liveTap        *liveTap // persistent live-stream tap for this server, if open
	liveTapDialing bool     // a tap dial is in flight; guards against a second
	// concurrent dial racing on the registry (two InitGames in quick succession)

	// Trinity handshake state
	trinityNonces    map[int]string           // map[clientNum]nonce
	pendingGreetings map[int]*pendingGreeting // map[clientNum]greeting awaiting handshake
}

// gauntletVictim is the last gauntlet victim, named in the humiliation award.
type gauntletVictim struct {
	name string
	guid string
}

// clientState holds only what the collector needs to correlate log
// events with outgoing envelopes; identity itself lives in the hub.
type clientState struct {
	clientID           int
	name               string
	cleanName          string
	guid               string
	model              string // player model, such as "sarge/krusade"
	isBot              bool
	isVR               bool
	vrSeen             bool // ran VR at any point in this stint
	isTrinityEngine    bool
	trinityUserType    int
	skill              float64 // bot skill level (1-5), 0 if human
	team               int
	joinedAt           time.Time
	ipAddress          string          // client IP address from ClientConnect
	began              bool            // true after ClientBegin (actually entered the game)
	frags              int             // frags accumulated this session (flushed on leave/match end)
	deaths             int             // deaths accumulated this session (flushed on leave/match end)
	impressives        int             // impressive awards this match
	excellents         int             // excellent awards this match
	humiliations       int             // gauntlet/humiliation awards this match
	defends            int             // defend awards this match
	captures           int             // flag captures this match
	flagReturns        int             // flag returns this match
	assists            int             // assist awards this match
	skullsDelivered    int             // Harvester: skulls deposited at enemy obelisk this match (persisted + live)
	obelisksDestroyed  int             // Overload: obelisks destroyed this match (persisted + live)
	skullsCarrying     int             // Harvester: skulls currently held (live only, not persisted)
	score              *int            // final score from score event at match end (nil if left early)
	lastGauntletVictim *gauntletVictim // last gauntlet kill victim (for humiliation award)
}

func NewServerManager(cfg *config.Config, server hub.ServerClient, rpc hub.RPCClient, pub hub.FactPublisher) *ServerManager {
	return &ServerManager{
		cfg:      cfg,
		server:   server,
		rpc:      rpc,
		pub:      pub,
		q3client: NewQ3Client(),
		events:   make(chan domain.Event, 100),
		servers:  make(map[int64]*serverState),
		tailers:  make(map[int64]*LogTailer),
		done:     make(chan struct{}),
	}
}

// SetLivePublisher tees live events onto NATS in collector-only mode. A
// combined hub and collector leaves it unset to avoid a self-loop.
func (m *ServerManager) SetLivePublisher(p hub.LiveEventPublisher) {
	m.livePub = p
}

// SetLiveRegistry injects the live-stream relay registry. When set, the
// collector opens a tap to each co-located game server when a match goes live
// and registers the resulting Buffer so the relay can serve it.
func (m *ServerManager) SetLiveRegistry(r *livestream.Registry) {
	m.liveReg = r
}

// SetConsoleRegistry injects the console-line registry. When set, Start
// spawns a persistent tap consumer per configured server (sv_conTap).
func (m *ServerManager) SetConsoleRegistry(r *console.Registry) {
	m.consoleReg = r
}

// SetRegistrarKick wires the heartbeat registrar's Kick so the manager can
// request an immediate is_live republish when a live tap opens or closes. It
// writes under m.mu because the dial goroutine reads kickRegistrar under that
// lock, so the callback must not block or take m.mu (registrar.Kick is a
// non-blocking channel send). Until it's wired, the periodic heartbeat carries
// is_live.
func (m *ServerManager) SetRegistrarKick(fn func()) {
	m.mu.Lock()
	m.kickRegistrar = fn
	m.mu.Unlock()
}

// openLiveTap opens a persistent live-stream tap for the server when the live
// registry is configured. Once a tap is open or a dial is in flight it does
// nothing, so two quick InitGames dial once. A server without sv_tvLive refuses
// the connection, which is only logged. The dial runs in a goroutine so a slow
// connect never stalls log processing. Callers (handleLogEvent) hold m.mu.
func (m *ServerManager) openLiveTap(state *serverState) {
	if m.liveReg == nil || state.match == nil || state.match.UUID == "" {
		return
	}
	if state.liveTap != nil || state.liveTapDialing {
		return // tap open, or a dial is already in flight
	}
	_, portStr, err := net.SplitHostPort(state.server.Address)
	if err != nil {
		log.Printf("collector: live tap: bad server address %q: %v", state.server.Address, err)
		return
	}
	// Stop() closes m.done before it waits on m.wg. Checking m.done and calling
	// wg.Add(1) under m.mu orders the Add before Stop()'s wg.Wait, so the dial
	// is always awaited.
	select {
	case <-m.done:
		return
	default:
	}
	state.liveTapDialing = true
	addr := net.JoinHostPort("127.0.0.1", portStr)
	key := state.server.Key
	serverID := state.server.ID
	delay := m.cfg.Tracker.Collector.LiveDelay.D()
	hdrTimeout := m.liveTapHdrTimeout
	if hdrTimeout <= 0 {
		hdrTimeout = defaultLiveTapHeaderTimeout
	}

	// Connect off the lock, then store the handle under it before starting the
	// consume loop, so onClose can't fire before the handle is visible. m.wg
	// tracks the dial so Stop() waits for one parked in the header read.
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.dialStoreStart(state, addr, key, serverID, hdrTimeout, delay)
	}()
}

// dialStoreStart performs one live-tap dial. On success it stores the handle
// and starts the consume loop under the lock and returns true. On any failure
// (dial or header error, shutdown during the dial, server removed) it stops the
// late tap and returns false. Every path clears state.liveTapDialing exactly
// once. Runs off m.mu.
func (m *ServerManager) dialStoreStart(state *serverState, addr, key string, serverID int64, hdrTimeout, delay time.Duration) bool {
	conn, err := dialLiveTap(addr)
	if err != nil {
		m.mu.Lock()
		state.liveTapDialing = false
		m.mu.Unlock()
		log.Printf("collector: live tap to %s for key=%s not available: %v", addr, key, err)
		return false
	}
	// Stop() closes registered conns to interrupt a parked header read. Bail if
	// shutdown already began.
	m.mu.Lock()
	select {
	case <-m.done:
		state.liveTapDialing = false
		m.mu.Unlock()
		conn.Close()
		return false
	default:
	}
	if m.inflightDials == nil {
		m.inflightDials = make(map[net.Conn]struct{})
	}
	m.inflightDials[conn] = struct{}{}
	m.mu.Unlock()

	tap, err := parseLiveTapHeader(conn, addr, key, m.liveReg, hdrTimeout, delay)

	m.mu.Lock()
	delete(m.inflightDials, conn)
	state.liveTapDialing = false
	if err != nil {
		m.mu.Unlock()
		log.Printf("collector: live tap to %s for key=%s not available: %v", addr, key, err)
		return false
	}
	// Shutdown came during the dial: stop the late tap before it registers a
	// buffer or starts a consumer.
	select {
	case <-m.done:
		m.mu.Unlock()
		tap.stop()
		return false
	default:
	}
	// The server may have been removed (reconfigured) while we were dialing.
	if _, ok := m.servers[serverID]; !ok {
		m.mu.Unlock()
		tap.stop()
		return false
	}
	state.liveTap = tap
	kick := m.kickRegistrar // capture under the lock; call after unlock
	// m.wg tracks the consume goroutine so Stop() waits for its teardown
	// (RemoveIf and onClose). The dial goroutine running this is still counted
	// in m.wg, so the counter can't have reached zero.
	m.wg.Add(1)
	m.mu.Unlock()
	tap.start(func(closed *liveTap) {
		defer m.wg.Done()
		m.onLiveTapClosed(state, closed)
	})
	if kick != nil {
		kick() // publish-on-change: flip is_live fast
	}
	log.Printf("collector: live tap open for key=%s via %s", key, addr)
	return true
}

const (
	// defaultLiveTapRedialBase is the first redial backoff after a mid-match tap
	// drop; it doubles each failed attempt up to liveTapRedialMaxBackoff.
	defaultLiveTapRedialBase = 250 * time.Millisecond
	// liveTapRedialMaxBackoff caps the redial interval. Redials continue for the
	// life of the match and stop when the match ends, the tap returns, another
	// dialer takes over, or the collector shuts down.
	liveTapRedialMaxBackoff = 30 * time.Second
)

// onLiveTapClosed runs once when a tap's connection drops. It clears the
// handle, only if it still holds this tap since a redial may have installed a
// newer one, and republishes is_live. If the match is still active and the
// collector isn't shutting down, the drop happened mid-match, so it starts a
// capped-backoff redial.
func (m *ServerManager) onLiveTapClosed(state *serverState, closed *liveTap) {
	m.mu.Lock()
	if state.liveTap != closed {
		m.mu.Unlock()
		return
	}
	state.liveTap = nil
	kick := m.kickRegistrar
	// The m.done check and wg.Add order the Add before Stop()'s wg.Wait, as in
	// openLiveTap. The loop claims liveTapDialing per attempt, so an InitGame
	// during a backoff sleep can take over and the loop yields.
	redial := false
	if state.match != nil && state.match.UUID != "" && !m.isShuttingDown() {
		redial = true
		m.wg.Add(1)
	}
	m.mu.Unlock()
	if kick != nil {
		kick()
	}
	if redial {
		go m.redialLiveTap(state)
	}
}

// redialLiveTap reopens a tap dropped mid-match, with capped exponential
// backoff, until the match ends. Each attempt claims the in-flight guard under
// the lock, yielding if the match ended, a tap is back, another dial is in
// flight, or the collector is shutting down.
func (m *ServerManager) redialLiveTap(state *serverState) {
	defer m.wg.Done()
	backoff := m.liveTapRedialBase
	if backoff <= 0 {
		backoff = defaultLiveTapRedialBase
	}
	for {
		select {
		case <-m.done:
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > liveTapRedialMaxBackoff {
			backoff = liveTapRedialMaxBackoff
		}
		m.mu.Lock()
		if m.isShuttingDown() || state.match == nil || state.match.UUID == "" ||
			state.liveTap != nil || state.liveTapDialing {
			m.mu.Unlock()
			return // match ended, tap already back, or another dialer took over
		}
		_, portStr, err := net.SplitHostPort(state.server.Address)
		if err != nil {
			m.mu.Unlock()
			log.Printf("collector: live tap redial: bad server address %q: %v", state.server.Address, err)
			return
		}
		state.liveTapDialing = true
		addr := net.JoinHostPort("127.0.0.1", portStr)
		key := state.server.Key
		serverID := state.server.ID
		delay := m.cfg.Tracker.Collector.LiveDelay.D()
		hdrTimeout := m.liveTapHdrTimeout
		if hdrTimeout <= 0 {
			hdrTimeout = defaultLiveTapHeaderTimeout
		}
		m.mu.Unlock()
		if m.dialStoreStart(state, addr, key, serverID, hdrTimeout, delay) {
			return // live is back
		}
		// dialStoreStart cleared liveTapDialing; back off and try again.
	}
}

// isShuttingDown reports whether Stop() has begun (m.done closed). A nil m.done
// (some test fixtures) reads as not shutting down.
func (m *ServerManager) isShuttingDown() bool {
	select {
	case <-m.done:
		return true
	default:
		return false
	}
}

// closeLiveTap tears down the server's live tap at collector shutdown or server
// removal; the tap itself persists across map rotation. The run loop
// deregisters its buffer when the connection closes, and clearing the handle
// here flips is_live at once. Callers hold m.mu.
func (m *ServerManager) closeLiveTap(state *serverState) {
	if state.liveTap != nil {
		state.liveTap.stop()
		state.liveTap = nil
		if m.kickRegistrar != nil {
			m.kickRegistrar() // publish-on-change
		}
	}
}

// SetReplayCutoffs sets the per-server replay/live boundaries; see cutoffFor.
func (m *ServerManager) SetReplayCutoffs(perServer map[int64]time.Time) {
	cuts := make(map[int64]time.Time, len(perServer))
	for id, ts := range perServer {
		cuts[id] = ts.UTC()
	}
	m.replayCutoffs = cuts
}

// Events returns the event channel for WebSocket broadcasting.
func (m *ServerManager) Events() <-chan domain.Event {
	return m.events
}

// Start initializes all servers and begins polling.
func (m *ServerManager) Start(ctx context.Context) error {
	source := ""
	if m.cfg.Tracker != nil && m.cfg.Tracker.Collector != nil {
		source = m.cfg.Tracker.Collector.SourceID
	}
	for _, srv := range m.cfg.Q3Servers {
		fullSrv, err := m.server.RegisterServer(ctx, source, srv.Key, srv.Address)
		if err != nil {
			return err
		}

		m.servers[fullSrv.ID] = &serverState{
			server:        *fullSrv,
			clients:       make(map[int]*clientState),
			trinityNonces: make(map[int]string),
			openSessions:  make(map[string]bool),
		}

		// Console tap consumer: persistent, per server (sv_conTap).
		if m.consoleReg != nil {
			if _, portStr, err := net.SplitHostPort(srv.Address); err == nil {
				runner := newConTapRunner(srv.Key, portStr, m.consoleReg.Ring(srv.Key), m.q3client, m.done)
				m.wg.Add(1)
				go func() {
					defer m.wg.Done()
					runner.run()
				}()
			} else {
				log.Printf("collector: console tap: bad server address %q: %v", srv.Address, err)
			}
		}

		// Serial replay: concurrent tailers fight for the SQLite write lock.
		if srv.LogPath != "" {
			startAfter := m.cutoffFor(&srv, fullSrv)
			serverID := fullSrv.ID
			if !m.attachTailer(ctx, srv.Key, srv.LogPath, serverID, startAfter) {
				// trinity.service can start before quake3-server@.service
				// creates the log, so poll for it in the background and
				// attach when it appears.
				log.Printf("Log file for %s not yet available (%s); retrying in background", srv.Key, srv.LogPath)
				m.wg.Add(1)
				go m.tailWhenReady(ctx, srv.Key, srv.LogPath, serverID, startAfter)
			}
		}
	}

	m.mu.Lock()
	m.startupComplete = true
	m.mu.Unlock()
	log.Printf("Startup complete, !link commands now enabled")

	return nil
}

// freshLogThreshold separates new logs (replay everything as live) from the
// log of a long-running server the collector was just added to (skip
// history). /etc/logrotate.d/quake3 caps the active file at about a day of
// activity, so a log over 10 MB comes from a server that has run too long
// to backfill on the hub.
const freshLogThreshold = 10 << 20 // 10 MB

// cutoffFor picks the replay cutoff for one server, in order:
//  1. m.replayCutoffs[id], the last event the publisher sent for this
//     server. Another server's activity can't move it.
//  2. fullSrv.LastMatchEndedAt, the hub's per-server watermark, when the
//     publisher has no entry (newly added, or idle since the last write).
//  3. The log's size: a small log replays everything; a large one skips
//     history (cutoff = now).
func (m *ServerManager) cutoffFor(srvCfg *config.Q3Server, fullSrv *domain.Server) time.Time {
	if ts, ok := m.replayCutoffs[fullSrv.ID]; ok && !ts.IsZero() {
		return ts
	}
	if fullSrv.LastMatchEndedAt != nil {
		return *fullSrv.LastMatchEndedAt
	}
	if info, err := os.Stat(srvCfg.LogPath); err == nil && info.Size() >= freshLogThreshold {
		return time.Now().UTC()
	}
	return time.Time{} // epoch: replay everything as live
}

// bootstrapServerPresence publishes a PresenceSnapshot for every client on
// one server that has begun, once replay has filled state.clients. The hub
// overwrites the slot entry, so repeats are harmless. It snapshots under the
// lock and publishes unlocked, so a NATS publish never holds up Stop or the
// retry goroutines.
func (m *ServerManager) bootstrapServerPresence(serverID int64) {
	m.mu.RLock()
	state, ok := m.servers[serverID]
	if !ok {
		m.mu.RUnlock()
		return
	}
	type pendingSnapshot struct {
		ts   time.Time
		data domain.PresenceSnapshotData
	}
	pending := make([]pendingSnapshot, 0, len(state.clients))
	skipped := 0
	for _, client := range state.clients {
		if client.guid == "" || !client.began {
			skipped++
			continue
		}
		state.openSessions[client.guid] = true
		pending = append(pending, pendingSnapshot{
			ts: client.joinedAt,
			data: domain.PresenceSnapshotData{
				GUID:            client.guid,
				Name:            client.name,
				CleanName:       client.cleanName,
				Model:           client.model,
				IsBot:           client.isBot,
				IsVR:            client.isVR,
				Skill:           client.skill,
				ClientNum:       client.clientID,
				Impressives:     client.impressives,
				Excellents:      client.excellents,
				Humiliations:    client.humiliations,
				Defends:         client.defends,
				Captures:        client.captures,
				Assists:         client.assists,
				SkullsDelivered: client.skullsDelivered,
			},
		})
	}
	m.mu.RUnlock()
	for _, p := range pending {
		m.pub.Publish(domain.FactEvent{
			Type:      domain.FactPresenceSnapshot,
			ServerID:  serverID,
			Timestamp: p.ts,
			Data:      p.data,
		})
	}
	log.Printf("collector: presence bootstrap server=%d published=%d skipped=%d", serverID, len(pending), skipped)
}

// BootstrapAll republishes presence snapshots for every tracked server. It
// runs after a NATS reconnect, which almost always means the hub restarted
// (its embedded NATS goes down with it) with an empty presence map.
//
// IDs are snapshotted under the lock and iterated unlocked, because
// bootstrapServerPresence takes m.mu.RLock itself.
func (m *ServerManager) BootstrapAll() {
	m.mu.RLock()
	ids := make([]int64, 0, len(m.servers))
	for id := range m.servers {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	for _, id := range ids {
		m.bootstrapServerPresence(id)
	}
}

// Stop stops all polling and log watching.
func (m *ServerManager) Stop() {
	log.Println("ServerManager: stopping...")
	close(m.done)
	// tailWhenReady may write m.tailers concurrently.
	m.mu.RLock()
	tailers := make([]*LogTailer, 0, len(m.tailers))
	for _, t := range m.tailers {
		tailers = append(tailers, t)
	}
	m.mu.RUnlock()
	for _, tailer := range tailers {
		tailer.Stop()
	}
	// Close in-flight dial conns too, so a goroutine parked in the header parse
	// unblocks at once.
	m.mu.Lock()
	for _, state := range m.servers {
		m.closeLiveTap(state)
	}
	for conn := range m.inflightDials {
		conn.Close()
	}
	m.mu.Unlock()
	m.wg.Wait()
	log.Println("ServerManager: shutdown complete")
}

// attachTailer opens the log, replays from startAfter, publishes a presence
// snapshot for the replayed clients, and starts the live tail. It returns
// false if the log can't be opened, so the caller can retry. Later errors are
// only logged.
//
// The snapshot goes out after replay and before the live tail starts, so the
// hub sees a consistent slot state before live updates arrive.
func (m *ServerManager) attachTailer(ctx context.Context, key, path string, serverID int64, startAfter time.Time) bool {
	tailer := NewLogTailer(path, nil)
	if _, err := tailer.OpenFile(); err != nil {
		return false
	}
	log.Printf("Replaying log for %s from %v", key, startAfter)
	if err := tailer.ReplayFromTimestamp(startAfter, func(event LogEvent, replayMode bool) {
		m.handleLogEvent(ctx, serverID, event, replayMode)
	}); err != nil {
		log.Printf("Warning: failed to replay log for %s: %v", key, err)
	}
	m.bootstrapServerPresence(serverID)
	if err := tailer.Start(); err != nil {
		log.Printf("Warning: failed to start log tailer for %s: %v", key, err)
		tailer.Stop()
		return true // we tried; don't ask the caller to retry forever
	}
	m.mu.Lock()
	// Stop() may have run since Start() above, and its tailer snapshot
	// missed this one.
	select {
	case <-m.done:
		m.mu.Unlock()
		tailer.Stop()
		return true
	default:
	}
	m.tailers[serverID] = tailer
	// If the replayed log shows a match in progress (the collector restarted
	// while the game server kept running), tap it now. The engine sends the
	// header and a keyframe to a consumer that joins mid-match, so viewers
	// reconnect within the delay window. openLiveTap does nothing without an
	// active match or a relay.
	if st, ok := m.servers[serverID]; ok {
		m.openLiveTap(st)
	}
	m.mu.Unlock()
	m.wg.Add(1)
	go m.processLogEvents(ctx, serverID, tailer)
	return true
}

// tailWhenReady polls for the log file and attaches the tailer as soon as
// the game server creates it.
func (m *ServerManager) tailWhenReady(ctx context.Context, key, path string, serverID int64, startAfter time.Time) {
	defer m.wg.Done()
	const interval = 3 * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.done:
			return
		case <-ticker.C:
		}
		if m.attachTailer(ctx, key, path, serverID, startAfter) {
			return
		}
	}
}

// ExecuteRcon sends an rcon command to a server and returns the reply.
func (m *ServerManager) ExecuteRcon(serverID int64, command string) (string, error) {
	m.mu.RLock()
	state, ok := m.servers[serverID]
	m.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("server not found")
	}

	var rconPassword string
	for _, srv := range m.cfg.Q3Servers {
		if srv.Address == state.server.Address {
			rconPassword = srv.RconPassword
			break
		}
	}

	if rconPassword == "" {
		return "", fmt.Errorf("RCON not configured for this server")
	}

	return m.q3client.RconCommand(state.server.Address, rconPassword, command)
}

// HasRconAccess reports whether a server has an rcon password configured.
func (m *ServerManager) HasRconAccess(serverID int64) bool {
	m.mu.RLock()
	state, ok := m.servers[serverID]
	m.mu.RUnlock()

	if !ok {
		return false
	}

	for _, srv := range m.cfg.Q3Servers {
		if srv.Address == state.server.Address && srv.RconPassword != "" {
			return true
		}
	}
	return false
}

// ExecuteRconByKey runs an rcon command on the server with this
// collector-local key. The NATS rcon proxy forwards the key and command
// from the hub.
func (m *ServerManager) ExecuteRconByKey(key, command string) (string, error) {
	m.mu.RLock()
	var address string
	for _, state := range m.servers {
		if strings.EqualFold(state.server.Key, key) {
			address = state.server.Address
			break
		}
	}
	m.mu.RUnlock()
	if address == "" {
		return "", fmt.Errorf("server %q not found", key)
	}
	var rconPassword string
	for _, srv := range m.cfg.Q3Servers {
		if srv.Address == address {
			rconPassword = srv.RconPassword
			break
		}
	}
	if rconPassword == "" {
		return "", fmt.Errorf("rcon not configured for server %q", key)
	}
	return m.q3client.RconCommand(address, rconPassword, command)
}

// AdminDelegationFor reports whether the operator opted the server with this
// key (case-insensitive) in to hub-admin rcon delegation.
func (m *ServerManager) AdminDelegationFor(key string) bool {
	m.mu.RLock()
	var address string
	for _, state := range m.servers {
		if strings.EqualFold(state.server.Key, key) {
			address = state.server.Address
			break
		}
	}
	m.mu.RUnlock()
	if address == "" {
		return false
	}
	for _, srv := range m.cfg.Q3Servers {
		if srv.Address == address {
			return srv.AllowHubAdminRcon
		}
	}
	return false
}

// Roster returns the registered servers for the Registrar's heartbeat.
// AdminDelegationEnabled mirrors the per-server config flag so the hub UI
// knows whether to offer a hub admin rcon.
func (m *ServerManager) Roster() []domain.RegdServer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// cfg.Q3Servers and m.servers share Address as their key.
	delegationByAddress := make(map[string]bool, len(m.cfg.Q3Servers))
	for _, srv := range m.cfg.Q3Servers {
		delegationByAddress[srv.Address] = srv.AllowHubAdminRcon
	}
	// The collector-wide target viewer delay (encode plus holdback), stamped on
	// every server so the live player can show it. The target already includes
	// the encode latency the holdback is derived from, so it's close to the
	// end-to-end delay.
	delaySec := 0
	if m.cfg.Tracker != nil && m.cfg.Tracker.Collector != nil {
		delaySec = int(m.cfg.Tracker.Collector.LiveDelay.D().Seconds())
	}
	out := make([]domain.RegdServer, 0, len(m.servers))
	for _, state := range m.servers {
		viewers := 0
		if m.liveReg != nil {
			viewers = m.liveReg.Viewers(state.server.Key)
		}
		out = append(out, domain.RegdServer{
			LocalID:                state.server.ID,
			Key:                    state.server.Key,
			Address:                state.server.Address,
			AdminDelegationEnabled: delegationByAddress[state.server.Address],
			IsLive:                 state.liveTap != nil,
			LiveDelaySeconds:       delaySec,
			LiveViewers:            viewers,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LocalID < out[j].LocalID })
	return out
}

func (m *ServerManager) processLogEvents(ctx context.Context, serverID int64, tailer *LogTailer) {
	defer m.wg.Done()

	for {
		select {
		case <-m.done:
			return
		case err := <-tailer.Errors:
			log.Printf("Log tailer error for server %d: %v", serverID, err)
		case event := <-tailer.Events:
			m.handleLogEvent(ctx, serverID, event, false) // live events are never replay mode
		}
	}
}

// handleLogEvent processes one log event. In replayMode, used at startup to
// rebuild client state from already-published lines, it updates only
// in-memory state.
func (m *ServerManager) handleLogEvent(ctx context.Context, serverID int64, event LogEvent, replayMode bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.servers[serverID]
	if !ok {
		return
	}

	switch event.Type {
	case EventTypeInitGame:
		data := event.Data.(InitGameData)
		m.handleMatchChange(ctx, state, data.MapName, data.GameType, data.UUID, data.Settings["g_mode"], data.Settings["g_trinityhandshake"] == "1", event.Timestamp, replayMode)
		// Open the tap at InitGame so warmup streams too: the engine streams from
		// spawn (sv_tvLive), separately from .tvd recording.
		if !replayMode && state.match != nil && state.match.UUID != "" {
			m.openLiveTap(state)
		}

	case EventTypeWarmupEnd:
		if state.match != nil {
			state.match.StartedAt = event.Timestamp
			if state.match.UUID != "" {
				switch {
				case replayMode && state.handshakeRequired:
					// Let a post-restart Shutdown fire match_end against
					// the hub's existing (pre-watermark) match row.
					state.matchStarted = true
				case !replayMode:
					// match_start goes out whatever g_trinityHandshake is, so
					// the hub's handshake_required column follows the cvar both
					// ways and the server's visibility updates by itself. The
					// hub keeps stats only from handshake matches. matchStarted
					// is set only when the hub created a match row, so match_end
					// always names a match the hub has.
					m.pub.Publish(domain.FactEvent{
						Type:      domain.FactMatchStart,
						ServerID:  state.server.ID,
						Timestamp: event.Timestamp,
						Data: domain.MatchStartData{
							MatchUUID:         state.match.UUID,
							MapName:           state.match.MapName,
							GameType:          state.match.GameType,
							Mode:              state.match.Mode,
							StartedAt:         event.Timestamp,
							HandshakeRequired: state.handshakeRequired,
						},
					})
					if state.handshakeRequired {
						state.matchStarted = true
					} else {
						log.Printf("collector: match_start for %s on server %s/%s with g_trinityHandshake not enabled; the hub will reject its stats and downgrade the server", state.match.UUID, state.server.Source, state.server.Key)
					}
				}
			}
		}
		state.matchState = "active"

	case EventTypeWarmup:
		data := event.Data.(WarmupData)
		state.warmupDuration = data.Duration
		state.matchState = "warmup"

	case EventTypeMatchState:
		data := event.Data.(MatchStateData)
		state.matchState = data.State
		if data.State == "warmup" && data.Duration > 0 {
			state.warmupDuration = data.Duration
		}

		// Intermission ends the match. As on the shutdown path, match_end needs
		// a match row from match_start. Replayed matches already sent their
		// match_end.
		if data.State == "intermission" && !replayMode && state.pendingExit != nil &&
			state.match != nil && state.match.UUID != "" &&
			state.matchStarted && !state.matchFlushed && state.match.EndedAt == nil {
			players := m.buildMatchEndPlayers(state, true)
			m.pub.Publish(domain.FactEvent{
				Type:      domain.FactMatchEnd,
				ServerID:  state.server.ID,
				Timestamp: state.pendingExitAt,
				Data: domain.MatchEndData{
					MatchUUID:  state.match.UUID,
					EndedAt:    state.pendingExitAt,
					ExitReason: *state.pendingExit,
					RedScore:   state.pendingRedScore,
					BlueScore:  state.pendingBlueScore,
					Players:    players,
				},
			})
			state.matchFlushed = true
			// The live stream keeps running: the intermission scoreboard is part
			// of the match, as in the recorded demo. The tap's buffer ends at the
			// engine's TVLe on the next map's spawn, after the scoreboard and the
			// delayed tail have streamed.
		}

	case EventTypeClientConnect:
		data := event.Data.(ClientConnectData)
		initialScore := 0
		state.clients[data.ClientID] = &clientState{
			clientID:  data.ClientID,
			joinedAt:  event.Timestamp,
			ipAddress: data.IPAddress,
			score:     &initialScore,
		}

	case EventTypeClientUserinfo:
		data := event.Data.(ClientUserinfoData)
		client, ok := state.clients[data.ClientID]
		if !ok {
			initialScore := 0
			client = &clientState{
				clientID: data.ClientID,
				joinedAt: event.Timestamp,
				score:    &initialScore,
			}
			state.clients[data.ClientID] = client
		}
		prevModel := client.model
		prevIsVR := client.isVR
		prevCleanName := client.cleanName
		wasBegan := client.began
		client.name = data.Name
		client.cleanName = domain.CleanQ3Name(data.Name)
		client.isBot = data.IsBot
		client.isVR = data.IsVR
		client.vrSeen = client.vrSeen || data.IsVR
		client.trinityUserType = data.TrinityUserType
		client.isTrinityEngine = data.IsTrinityEngine
		client.skill = data.Skill
		client.team = data.Team
		client.model = data.Model

		// Bots get a synthetic "BOT:<cleanName>" GUID; humans use the GUID
		// from the log. Empty until identity is known.
		if data.IsBot {
			client.guid = "BOT:" + client.cleanName
		} else {
			client.guid = data.GUID
		}

		// For a client already in the game, forward changes to userinfo
		// fields the hub tracks so the live card shows the new portrait or
		// VR flag.
		if !replayMode && wasBegan && client.guid != "" &&
			(prevModel != client.model || prevIsVR != client.isVR) {
			m.emitEvent(domain.Event{
				Type:      domain.EventClientUserinfo,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.ClientUserinfoEvent{
					ClientNum: data.ClientID,
					GUID:      client.guid,
					Model:     client.model,
					IsVR:      client.isVR,
				},
			})
		}
		// A switch into VR after joining; the join recorded the starting mode.
		if !replayMode && wasBegan && client.guid != "" && !client.isBot && client.isVR && !prevIsVR {
			m.pub.Publish(domain.FactEvent{
				Type:      domain.FactPlayerVR,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data:      domain.PlayerVRData{GUID: client.guid},
			})
		}

		// Replay skips the identity RPC: the hub's presence tracker answers
		// PlayerID, verified and admin, and hundreds of upserts would
		// dominate collector startup. A bot seen only before the watermark
		// and missing from the hub DB has its match_end stats dropped, with a
		// log line; it's upserted again the next time it joins.
		if client.guid != "" && !replayMode {
			var identity hub.PlayerIdentity
			var err error
			if data.IsBot {
				identity, err = m.server.UpsertBotPlayerIdentity(ctx, data.Name, client.cleanName, event.Timestamp)
			} else {
				identity, err = m.server.UpsertPlayerIdentity(ctx, data.GUID, data.Name, client.cleanName, event.Timestamp, data.IsVR)
			}
			if err != nil {
				log.Printf("Error resolving player identity for GUID %s: %v", client.guid, err)
			}

			if wasBegan && !data.IsBot && prevCleanName != client.cleanName && client.cleanName != "UnnamedPlayer" && identity.Found {
				var excludePlayerID int64
				if client.trinityUserType > 0 {
					excludePlayerID = identity.PlayerID
				}
				if reserved, err := m.server.IsNameReserved(ctx, data.Name, excludePlayerID); err == nil && reserved {
					m.sendTrinityNameConflict(serverID, data.ClientID)
				}
			}
		}

	case EventTypeClientBegin:
		data := event.Data.(ClientConnectData)
		if client, ok := state.clients[data.ClientID]; ok {
			client.began = true

			// A Begin is either a join or a map-change continuation;
			// state.openSessions mirrors the hub's sessions to tell them
			// apart, and the published fact and the greeting both follow.
			if client.guid != "" && !replayMode {
				if state.openSessions[client.guid] {
					m.pub.Publish(domain.FactEvent{
						Type:      domain.FactPresenceSnapshot,
						ServerID:  serverID,
						Timestamp: event.Timestamp,
						Data: domain.PresenceSnapshotData{
							GUID:            client.guid,
							Name:            client.name,
							CleanName:       client.cleanName,
							Model:           client.model,
							IsBot:           client.isBot,
							IsVR:            client.isVR,
							Skill:           client.skill,
							ClientNum:       data.ClientID,
							Impressives:     client.impressives,
							Excellents:      client.excellents,
							Humiliations:    client.humiliations,
							Defends:         client.defends,
							Captures:        client.captures,
							Assists:         client.assists,
							SkullsDelivered: client.skullsDelivered,
						},
					})
					if !client.isBot && client.cleanName != "UnnamedPlayer" {
						var excludePlayerID int64
						if client.trinityUserType > 0 {
							id, err := m.server.LookupPlayerIdentity(ctx, client.guid)
							if err == nil && id.Found {
								excludePlayerID = id.PlayerID
							}
						}
						if reserved, err := m.server.IsNameReserved(ctx, client.name, excludePlayerID); err == nil && reserved {
							m.sendTrinityNameConflict(serverID, data.ClientID)
						}
					}
				} else {
					state.openSessions[client.guid] = true
					m.pub.Publish(domain.FactEvent{
						Type:      domain.FactPlayerJoin,
						ServerID:  serverID,
						Timestamp: event.Timestamp,
						Data: domain.PlayerJoinData{
							GUID:      client.guid,
							Name:      client.name,
							CleanName: client.cleanName,
							Model:     client.model,
							IP:        client.ipAddress,
							IsBot:     client.isBot,
							IsVR:      client.isVR,
							Skill:     client.skill,
							JoinedAt:  event.Timestamp,
							ClientNum: data.ClientID,
						},
					})
					// Bots have a synthetic GUID; greet only humans.
					if !client.isBot && m.startupComplete {
						if m.handshakeRequired(state) {
							// The handshake handler greets with the auth proof.
							m.scheduleGreetingAfterHandshake(ctx, state, serverID, data.ClientID, client.guid, client.name, client.cleanName, client.isVR, client.isTrinityEngine)
						} else {
							go m.performGreet(ctx, serverID, data.ClientID, client.guid, client.name, client.cleanName, client.isVR, client.isTrinityEngine, nil)
						}
					}
				}
			}

			if !replayMode {
				m.emitEvent(domain.Event{
					Type:      domain.EventPlayerJoin,
					ServerID:  serverID,
					Timestamp: event.Timestamp,
					Data: domain.PlayerJoinEvent{
						Player: domain.PlayerStatus{
							GUID:      client.guid,
							Name:      client.name,
							CleanName: client.cleanName,
							IsBot:     client.isBot,
							IsVR:      client.isVR,
							Team:      client.team,
							JoinedAt:  event.Timestamp,
							ClientNum: data.ClientID,
						},
					},
				})
			}
		}

	case EventTypeClientDisconnect:
		data := event.Data.(ClientDisconnectData)
		if client, ok := state.clients[data.ClientID]; ok {
			if !client.isBot && client.guid != "" && !replayMode {
				duration := int(event.Timestamp.Sub(client.joinedAt).Seconds())
				if duration < 0 {
					duration = 0
				}
				m.pub.Publish(domain.FactEvent{
					Type:      domain.FactPlayerLeave,
					ServerID:  serverID,
					Timestamp: event.Timestamp,
					Data: domain.PlayerLeaveData{
						GUID:            client.guid,
						ClientNum:       data.ClientID,
						LeftAt:          event.Timestamp,
						DurationSeconds: duration,
					},
				})
			}

			// Keep the leaving client's stats for the match-end flush.
			if !state.matchFlushed && client.began && client.guid != "" &&
				(state.matchState == "active" || state.matchState == "overtime") &&
				(client.team != 3 || client.frags > 0 || client.deaths > 0) {
				state.savePreviousClient(client)
			}

			if !replayMode {
				m.emitEvent(domain.Event{
					Type:      domain.EventPlayerLeave,
					ServerID:  serverID,
					Timestamp: event.Timestamp,
					Data: domain.PlayerLeaveEvent{
						PlayerName: client.name,
						GUID:       client.guid,
					},
				})
			}

			if client.guid != "" {
				delete(state.openSessions, client.guid)
			}
			delete(state.trinityNonces, data.ClientID)
			if state.pendingGreetings != nil {
				if pg, ok := state.pendingGreetings[data.ClientID]; ok {
					pg.timer.Stop()
					delete(state.pendingGreetings, data.ClientID)
				}
			}

			delete(state.clients, data.ClientID)
		}

	case EventTypeFrag:
		data := event.Data.(FragEventData)

		// Replay counts too, so an unfinished match can still be flushed.
		if state.matchState == "active" || state.matchState == "overtime" {
			if fragger, ok := state.clients[data.FraggerID]; ok {
				fragger.frags++
			}

			if victim, ok := state.clients[data.VictimID]; ok {
				victim.deaths++
			}

			// MOD_GAUNTLET is 2; the victim pairs with the humiliation award.
			if data.WeaponID == 2 {
				if fragger, ok := state.clients[data.FraggerID]; ok {
					var victimGUID string
					if victim, ok := state.clients[data.VictimID]; ok {
						victimGUID = victim.guid
					}
					fragger.lastGauntletVictim = &gauntletVictim{
						name: data.VictimName,
						guid: victimGUID,
					}
				}
			}
		}

		if !replayMode {
			var fraggerGUID, victimGUID string
			if fragger, ok := state.clients[data.FraggerID]; ok {
				fraggerGUID = fragger.guid
			}
			if victim, ok := state.clients[data.VictimID]; ok {
				victimGUID = victim.guid
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventFrag,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.FragEvent{
					Fragger:     data.FraggerName,
					Victim:      data.VictimName,
					Weapon:      data.Weapon,
					FraggerGUID: fraggerGUID,
					VictimGUID:  victimGUID,
				},
			})
		}

	case EventTypeScore:
		// Score events arrive during the Exit sequence with final scores and teams.
		data := event.Data.(ScoreEventData)
		if client, ok := state.clients[data.ClientID]; ok {
			score := data.Score
			client.score = &score
			client.team = data.Team
		}

	case EventTypeExit:
		data := event.Data.(ExitEventData)
		if state.match != nil {
			// Score events come after Exit and before ShutdownGame, so the
			// stats flush waits for ShutdownGame.
			reason := data.Reason
			state.pendingExit = &reason
			state.pendingExitAt = event.Timestamp

			state.pendingRedScore = data.RedScore
			state.pendingBlueScore = data.BlueScore

			if !replayMode {
				m.emitEvent(domain.Event{
					Type:      domain.EventMatchEnd,
					ServerID:  serverID,
					Timestamp: event.Timestamp,
					Data:      domain.MatchEndEvent{ExitReason: data.Reason},
				})
			}
		}

	case EventTypeShutdown:
		// A warmup InitGame is immediately followed by a fake
		// ShutdownGame at the same timestamp; drop it.
		if event.Timestamp.Equal(state.lastInitGame) {
			break
		}

		if state.match != nil {
			if state.matchFlushed || replayMode {
				// Already flushed, or a replayed match. The hub drops
				// boundary duplicates by source_progress.consumed_seq.
			} else if state.matchStarted && state.match.UUID != "" && state.match.EndedAt == nil {
				if state.pendingExit != nil {
					m.pub.Publish(domain.FactEvent{
						Type:      domain.FactMatchEnd,
						ServerID:  state.server.ID,
						Timestamp: state.pendingExitAt,
						Data: domain.MatchEndData{
							MatchUUID:  state.match.UUID,
							EndedAt:    state.pendingExitAt,
							ExitReason: *state.pendingExit,
							RedScore:   state.pendingRedScore,
							BlueScore:  state.pendingBlueScore,
							Players:    m.buildMatchEndPlayers(state, true),
						},
					})
				} else {
					// Abnormal shutdown: no Exit event, so no scores or victories
					m.pub.Publish(domain.FactEvent{
						Type:      domain.FactMatchEnd,
						ServerID:  state.server.ID,
						Timestamp: event.Timestamp,
						Data: domain.MatchEndData{
							MatchUUID:  state.match.UUID,
							EndedAt:    event.Timestamp,
							ExitReason: "shutdown",
							Players:    m.buildMatchEndPlayers(state, false),
						},
					})
				}
			} else if !state.matchStarted {
				// The match never left warmup.
				log.Printf("Discarding match %s: never reached active play", state.match.UUID)
			}
		}
		// The live tap stays open across the rotation (see liveTap).
		state.match = nil
		state.matchStarted = false
		state.matchFlushed = false
		state.pendingExit = nil
		state.pendingRedScore = nil
		state.pendingBlueScore = nil
		state.clients = make(map[int]*clientState)
		state.previousClients = make(map[string]*clientState)

	case EventTypeFlagCapture:
		data := event.Data.(FlagCaptureData)
		if client, ok := state.clients[data.ClientID]; ok {
			client.captures++
		}
		if !replayMode {
			var guid string
			if client, ok := state.clients[data.ClientID]; ok {
				guid = client.guid
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventFlagCapture,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.FlagCaptureEvent{
					ClientNum:  data.ClientID,
					PlayerName: data.Name,
					Team:       data.Team,
					GUID:       guid,
				},
			})
		}

	case EventTypeFlagTaken:
		data := event.Data.(FlagTakenData)
		if !replayMode {
			var guid string
			if client, ok := state.clients[data.ClientID]; ok {
				guid = client.guid
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventFlagTaken,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.FlagTakenEvent{
					ClientNum:  data.ClientID,
					PlayerName: data.Name,
					Team:       data.Team,
					GUID:       guid,
				},
			})
		}

	case EventTypeFlagReturn:
		data := event.Data.(FlagReturnData)
		// Automatic returns have ClientID -1; only player returns count.
		if data.ClientID >= 0 {
			if client, ok := state.clients[data.ClientID]; ok {
				client.flagReturns++
			}
		}
		if !replayMode {
			var guid string
			if data.ClientID >= 0 {
				if client, ok := state.clients[data.ClientID]; ok {
					guid = client.guid
				}
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventFlagReturn,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.FlagReturnEvent{
					ClientNum:  data.ClientID,
					PlayerName: data.Name,
					Team:       data.Team,
					GUID:       guid,
				},
			})
		}

	case EventTypeFlagDrop:
		data := event.Data.(FlagDropData)
		if !replayMode {
			var guid string
			if client, ok := state.clients[data.ClientID]; ok {
				guid = client.guid
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventFlagDrop,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.FlagDropEvent{
					ClientNum:  data.ClientID,
					PlayerName: data.Name,
					Team:       data.Team,
					GUID:       guid,
				},
			})
		}

	case EventTypeObeliskDestroy:
		data := event.Data.(ObeliskDestroyData)
		// World and self destructs have no player to credit.
		if data.AttackerID >= 0 {
			if client, ok := state.clients[data.AttackerID]; ok {
				client.obelisksDestroyed++
			}
		}
		if !replayMode {
			var guid string
			if data.AttackerID >= 0 {
				if client, ok := state.clients[data.AttackerID]; ok {
					guid = client.guid
				}
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventObeliskDestroy,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.ObeliskDestroyEvent{
					AttackerName: data.Attacker,
					Team:         data.Team,
					GUID:         guid,
				},
			})
		}

	case EventTypeObeliskDamage:
		data := event.Data.(ObeliskDamageData)
		// The mod sends one Start and one Stop per attack burst.
		if replayMode || data.AttackerID < 0 {
			break
		}
		var guid string
		if client, ok := state.clients[data.AttackerID]; ok {
			guid = client.guid
		}
		m.emitEvent(domain.Event{
			Type:      domain.EventObeliskDamage,
			ServerID:  serverID,
			Timestamp: event.Timestamp,
			Data: domain.ObeliskDamageEvent{
				ClientNum:    data.AttackerID,
				AttackerName: data.Attacker,
				Team:         data.Team,
				Active:       data.Active,
				GUID:         guid,
			},
		})

	case EventTypeSkullPickup:
		data := event.Data.(SkullPickupData)
		// data.Count is the running total after the pickup.
		if client, ok := state.clients[data.ClientID]; ok {
			client.skullsCarrying = data.Count
		}
		if !replayMode {
			var guid string
			if client, ok := state.clients[data.ClientID]; ok {
				guid = client.guid
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventSkullPickup,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.SkullPickupEvent{
					ClientNum:  data.ClientID,
					PlayerName: data.Name,
					Team:       data.Team,
					Count:      data.Count,
					GUID:       guid,
				},
			})
		}

	case EventTypeSkullScore:
		data := event.Data.(SkullScoreData)
		// data.Skulls is the count delivered on this trip; delivery clears the
		// live carry count.
		if client, ok := state.clients[data.ClientID]; ok {
			client.skullsDelivered += data.Skulls
			client.skullsCarrying = 0
		}
		if !replayMode {
			var guid string
			if client, ok := state.clients[data.ClientID]; ok {
				guid = client.guid
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventSkullScore,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.SkullScoreEvent{
					PlayerName: data.Name,
					Team:       data.Team,
					Skulls:     data.Skulls,
					GUID:       guid,
				},
			})
		}

	case EventTypeTeamChange:
		data := event.Data.(TeamChangeData)
		if client, ok := state.clients[data.ClientID]; ok {
			oldTeam := client.team

			// Leaving a playing team keeps the stats for the match-end flush.
			if oldTeam != 3 && oldTeam != data.NewTeam && client.guid != "" {
				if state.matchStarted &&
					(state.matchState == "active" || state.matchState == "overtime") {
					state.savePreviousClient(client)

					// A fresh clientState for the new team, with the same identity.
					initialScore := 0
					state.clients[data.ClientID] = &clientState{
						clientID:  client.clientID,
						name:      client.name,
						cleanName: client.cleanName,
						guid:      client.guid,
						model:     client.model,
						isBot:     client.isBot,
						isVR:      client.isVR,
						vrSeen:    client.isVR,
						skill:     client.skill,
						team:      data.NewTeam,
						joinedAt:  event.Timestamp,
						ipAddress: client.ipAddress,
						began:     client.began,
						score:     &initialScore,
					}
				} else {
					client.team = data.NewTeam
				}
			} else {
				client.team = data.NewTeam
				// joinedAt restarts when a spectator joins a team.
				if data.NewTeam != 3 && oldTeam == 3 {
					client.joinedAt = event.Timestamp
				}
			}
		}

		if !replayMode {
			var guid string
			if client, ok := state.clients[data.ClientID]; ok {
				guid = client.guid
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventTeamChange,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.TeamChangeEvent{
					PlayerName: data.Name,
					OldTeam:    data.OldTeam,
					NewTeam:    data.NewTeam,
					GUID:       guid,
				},
			})
		}

	case EventTypeAssist:
		if state.matchState != "active" && state.matchState != "overtime" {
			break
		}
		data := event.Data.(AssistData)
		if client, ok := state.clients[data.ClientID]; ok {
			client.assists++
			if !replayMode {
				m.emitEvent(domain.Event{
					Type:      domain.EventAward,
					ServerID:  serverID,
					Timestamp: event.Timestamp,
					Data: domain.AwardEvent{
						ClientNum:  data.ClientID,
						PlayerName: data.Name,
						AwardType:  "assist",
						AssistType: data.AssistType,
						Team:       client.team,
						GUID:       client.guid,
					},
				})
			}
		}

	case EventTypeAward:
		if state.matchState != "active" && state.matchState != "overtime" {
			break
		}
		data := event.Data.(AwardData)
		if client, ok := state.clients[data.ClientID]; ok {
			switch data.AwardType {
			case "impressive":
				client.impressives++
			case "excellent":
				client.excellents++
			case "gauntlet":
				client.humiliations++
			case "defend":
				client.defends++
			}

			if !replayMode {
				// The site calls the gauntlet award humiliation.
				awardType := data.AwardType
				if awardType == "gauntlet" {
					awardType = "humiliation"
				}

				awardEvent := domain.AwardEvent{
					ClientNum:  data.ClientID,
					PlayerName: data.Name,
					AwardType:  awardType,
					Team:       client.team,
					GUID:       client.guid,
				}

				if data.AwardType == "gauntlet" && client.lastGauntletVictim != nil {
					awardEvent.VictimName = client.lastGauntletVictim.name
					awardEvent.VictimGUID = client.lastGauntletVictim.guid
					client.lastGauntletVictim = nil
				}

				m.emitEvent(domain.Event{
					Type:      domain.EventAward,
					ServerID:  serverID,
					Timestamp: event.Timestamp,
					Data:      awardEvent,
				})
			}
		}

	case EventTypeCommand:
		data := event.Data.(CommandData)
		// Commands are ignored during replay and until startup completes.
		if !replayMode && m.startupComplete {
			m.handleCommand(ctx, serverID, state, data.ClientID, data.Command)
		}

	case EventTypeSay:
		data := event.Data.(SayData)
		if !replayMode {
			var guid string
			if client, ok := state.clients[data.ClientID]; ok {
				guid = client.guid
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventSay,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.SayEvent{
					ClientNum:  data.ClientID,
					PlayerName: data.Name,
					Message:    data.Message,
					GUID:       guid,
				},
			})
		}

	case EventTypeSayTeam:
		data := event.Data.(SayTeamData)
		if !replayMode {
			var guid string
			if client, ok := state.clients[data.ClientID]; ok {
				guid = client.guid
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventSayTeam,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.SayTeamEvent{
					ClientNum:  data.ClientID,
					PlayerName: data.Name,
					Message:    data.Message,
					GUID:       guid,
				},
			})
		}

	case EventTypeTell:
		data := event.Data.(TellData)
		if !replayMode {
			var fromGUID, toGUID string
			if client, ok := state.clients[data.FromClientID]; ok {
				fromGUID = client.guid
			}
			if client, ok := state.clients[data.ToClientID]; ok {
				toGUID = client.guid
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventTell,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.TellEvent{
					FromClientNum: data.FromClientID,
					ToClientNum:   data.ToClientID,
					FromName:      data.FromName,
					ToName:        data.ToName,
					Message:       data.Message,
					FromGUID:      fromGUID,
					ToGUID:        toGUID,
				},
			})
		}

	case EventTypeSayRcon:
		data := event.Data.(SayRconData)
		if !replayMode {
			m.emitEvent(domain.Event{
				Type:      domain.EventSayRcon,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.SayRconEvent{
					Message: data.Message,
				},
			})
		}

	case EventTypeServerStartup:
		if !replayMode {
			m.pub.Publish(domain.FactEvent{
				Type:      domain.FactServerStartup,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data:      domain.ServerStartupData{StartedAt: event.Timestamp},
			})
		}

	case EventTypeServerShutdown:
		if !replayMode {
			m.pub.Publish(domain.FactEvent{
				Type:      domain.FactServerShutdown,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data:      domain.ServerShutdownData{ShutdownAt: event.Timestamp},
			})
		}

	case EventTypeDemoSaved:
		data := event.Data.(DemoSavedData)
		if data.MatchUUID != "" && !replayMode {
			m.pub.Publish(domain.FactEvent{
				Type:      domain.FactDemoFinalized,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.DemoFinalizedData{
					MatchUUID:  data.MatchUUID,
					Frames:     data.Frames,
					DurationMS: data.DurationMS,
					Bytes:      data.Bytes,
				},
			})
		}

	case EventTypeDemoDiscarded:
		// No fact: to the hub, a missing FactDemoFinalized means the same
		// thing. Logged for review.

	case EventTypeRconExec:
		data := event.Data.(RconExecData)
		// The engine already filters trinity_rconset, and the hub audits the
		// autoset path as rcon.autoset; drop any that gets through.
		if strings.HasPrefix(data.Command, "sv_cmd trinity_rconset") {
			break
		}
		if !replayMode {
			m.pub.Publish(domain.FactEvent{
				Type:      domain.FactRconExec,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.RconExecData{
					ClientAddr: data.ClientAddr,
					GUID:       data.GUID,
					Command:    data.Command,
				},
			})
		}

	case EventTypeRconDenied:
		data := event.Data.(RconDeniedData)
		if !replayMode {
			m.pub.Publish(domain.FactEvent{
				Type:      domain.FactRconDenied,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data:      domain.RconDeniedData{ClientAddr: data.ClientAddr},
			})
		}

	case EventTypeCvarChange:
		data := event.Data.(CvarChangeData)
		if state.match != nil && state.match.UUID != "" {
			switch data.Key {
			case "g_mode":
				state.match.Mode = data.Value
				m.pub.Publish(domain.FactEvent{
					Type:      domain.FactMatchSettingsUpdate,
					ServerID:  state.server.ID,
					Timestamp: event.Timestamp,
					Data:      domain.MatchSettingsUpdateData{MatchUUID: state.match.UUID, Mode: data.Value},
				})
			}
		}

	case EventTypeTrinityChallenge:
		if !replayMode {
			data := event.Data.(TrinityChallengeData)
			state.trinityNonces[data.ClientNum] = data.Nonce
		}

	case EventTypeTrinityHandshake:
		if !replayMode {
			data := event.Data.(TrinityHandshakeData)
			m.handleTrinityHandshake(ctx, serverID, state, data)
		}
	}
}

// handleMatchChange starts tracking the match an InitGame begins.
func (m *ServerManager) handleMatchChange(ctx context.Context, state *serverState, mapName string, gameType int, uuid string, mode string, handshakeEnabled bool, ts time.Time, replayMode bool) {
	state.handshakeRequired = handshakeEnabled
	// The game can log InitGame twice at one timestamp when a server restarts.
	if ts.Equal(state.lastInitGame) {
		return
	}
	state.lastInitGame = ts

	gameTypeStr := domain.GameTypeFromInt(gameType)

	// Replay only rebuilds in-memory state; the hub drops duplicate
	// publishes by consumed_seq and the publish watermark.
	if replayMode {
		prevMatch := state.match

		// Emit match_crashed for a prior match that started but never
		// ended. The hub's handler is idempotent.
		if prevMatch != nil && prevMatch.UUID != "" &&
			prevMatch.UUID != uuid && prevMatch.EndedAt == nil &&
			state.matchStarted {
			m.pub.Publish(domain.FactEvent{
				Type: domain.FactMatchCrashed, ServerID: state.server.ID, Timestamp: ts,
				Data: domain.MatchCrashedData{MatchUUID: prevMatch.UUID, EndedAt: ts},
			})
		}

		state.match = &domain.Match{
			UUID:      uuid,
			ServerID:  state.server.ID,
			MapName:   mapName,
			GameType:  gameTypeStr,
			StartedAt: ts,
			Mode:      mode,
		}
		state.matchStarted = false
		state.clients = make(map[int]*clientState)
		state.previousClients = make(map[string]*clientState)
		state.matchFlushed = false
		state.matchState = ""
		state.warmupDuration = 0
		return
	}

	// Close a previous unflushed match (no ShutdownGame) as crashed.
	if !state.matchFlushed && state.matchStarted && state.match != nil &&
		state.match.UUID != "" && state.match.EndedAt == nil {
		m.pub.Publish(domain.FactEvent{
			Type:      domain.FactMatchEnd,
			ServerID:  state.server.ID,
			Timestamp: ts,
			Data: domain.MatchEndData{
				MatchUUID:  state.match.UUID,
				EndedAt:    ts,
				ExitReason: "crashed",
				Players:    m.buildMatchEndPlayers(state, false),
			},
		})
	}

	// The match is persisted at WarmupEnd, so a warmup-only match leaves no
	// row.
	match := &domain.Match{
		UUID:      uuid,
		ServerID:  state.server.ID,
		MapName:   mapName,
		GameType:  gameTypeStr,
		StartedAt: ts,
		Mode:      mode,
	}
	state.match = match
	state.matchStarted = false

	state.clients = make(map[int]*clientState)
	state.previousClients = make(map[string]*clientState)
	state.matchFlushed = false
	state.matchState = "" // set by the MatchState event when warmup is on
	state.warmupDuration = 0

	m.emitEvent(domain.Event{
		Type:      domain.EventMatchStart,
		ServerID:  state.server.ID,
		Timestamp: ts,
		Data: domain.MatchStartEvent{
			Map:      mapName,
			GameType: gameTypeStr,
		},
	})
}

// savePreviousClient accumulates per-GUID counters from a completed stint.
func (state *serverState) savePreviousClient(client *clientState) {
	if state.previousClients == nil {
		state.previousClients = make(map[string]*clientState)
	}
	if client.guid == "" {
		return
	}
	if prev, ok := state.previousClients[client.guid]; ok {
		prev.frags += client.frags
		prev.deaths += client.deaths
		prev.captures += client.captures
		prev.flagReturns += client.flagReturns
		prev.assists += client.assists
		prev.impressives += client.impressives
		prev.excellents += client.excellents
		prev.humiliations += client.humiliations
		prev.defends += client.defends
		prev.skullsDelivered += client.skullsDelivered
		prev.obelisksDestroyed += client.obelisksDestroyed
		prev.clientID = client.clientID
		prev.team = client.team
		prev.model = client.model
		prev.vrSeen = prev.vrSeen || client.vrSeen
	} else {
		state.previousClients[client.guid] = client
	}
}

// buildMatchEndPlayers lays out previous stints first and connected
// players last, so connected-client metadata wins on duplicate GUIDs.
func (m *ServerManager) buildMatchEndPlayers(state *serverState, computeVictory bool) []domain.MatchEndPlayer {
	var maxFFAScore int
	var hasFFAScores bool
	if computeVictory {
		maxFFAScore, hasFFAScores = computeMaxScore(state.clients)
	}

	var players []domain.MatchEndPlayer

	// Previous stints (completed=false).
	for _, client := range state.previousClients {
		if client.guid == "" {
			continue
		}
		if client.team == 3 && client.frags == 0 && client.deaths == 0 {
			continue
		}
		var team *int
		if client.team > 0 {
			team = &client.team
		}
		joinedLate := state.match != nil && client.joinedAt.After(state.match.StartedAt)
		players = append(players, domain.MatchEndPlayer{
			GUID:              client.guid,
			ClientID:          client.clientID,
			Name:              client.name,
			CleanName:         client.cleanName,
			Frags:             client.frags,
			Deaths:            client.deaths,
			Completed:         false,
			Score:             client.score,
			Team:              team,
			Model:             client.model,
			Skill:             client.skill,
			Victory:           false,
			Captures:          client.captures,
			FlagReturns:       client.flagReturns,
			Assists:           client.assists,
			Impressives:       client.impressives,
			Excellents:        client.excellents,
			Humiliations:      client.humiliations,
			Defends:           client.defends,
			SkullsDelivered:   client.skullsDelivered,
			ObelisksDestroyed: client.obelisksDestroyed,
			IsBot:             client.isBot,
			JoinedLate:        joinedLate,
			JoinedAt:          client.joinedAt,
			IsVR:              client.isVR || client.vrSeen,
		})
	}

	// Connected players last (completed=true).
	for clientID, client := range state.clients {
		if client.guid == "" || !client.began {
			continue
		}
		var team *int
		if client.team > 0 {
			team = &client.team
		}
		var victory bool
		if computeVictory {
			victory = isMatchWinner(client, state, maxFFAScore, hasFFAScores)
		}
		joinedLate := state.match != nil && client.joinedAt.After(state.match.StartedAt)
		players = append(players, domain.MatchEndPlayer{
			GUID:              client.guid,
			ClientID:          clientID,
			Name:              client.name,
			CleanName:         client.cleanName,
			Frags:             client.frags,
			Deaths:            client.deaths,
			Completed:         true,
			Score:             client.score,
			Team:              team,
			Model:             client.model,
			Skill:             client.skill,
			Victory:           victory,
			Captures:          client.captures,
			FlagReturns:       client.flagReturns,
			Assists:           client.assists,
			Impressives:       client.impressives,
			Excellents:        client.excellents,
			Humiliations:      client.humiliations,
			Defends:           client.defends,
			SkullsDelivered:   client.skullsDelivered,
			ObelisksDestroyed: client.obelisksDestroyed,
			IsBot:             client.isBot,
			JoinedLate:        joinedLate,
			JoinedAt:          client.joinedAt,
			IsVR:              client.isVR || client.vrSeen,
		})
	}

	return players
}

// computeWinningTeam returns 1 for red, 2 for blue, or 0 for a tie or missing
// scores.
func computeWinningTeam(redScore, blueScore *int) int {
	if redScore != nil && blueScore != nil {
		if *redScore > *blueScore {
			return 1 // Red
		} else if *blueScore > *redScore {
			return 2 // Blue
		}
	}
	return 0 // Tie or unknown
}

// computeMaxScore returns the highest score among completed clients, for the
// FFA victory, and whether any client had a score.
func computeMaxScore(clients map[int]*clientState) (int, bool) {
	found := false
	maxScore := 0
	for _, client := range clients {
		if client.guid != "" && client.score != nil {
			if !found || *client.score > maxScore {
				maxScore = *client.score
				found = true
			}
		}
	}
	return maxScore, found
}

// isMatchWinner reports whether a client won the match.
func isMatchWinner(client *clientState, state *serverState, maxFFAScore int, hasFFAScores bool) bool {
	if state.match == nil || state.pendingExit == nil {
		return false // No victory for abnormal shutdown
	}

	gameType := state.match.GameType
	isTeamGame := gameType == domain.GameTypeCTF || gameType == domain.GameTypeTDM ||
		gameType == domain.GameType1FCTF || gameType == domain.GameTypeOverload ||
		gameType == domain.GameTypeHarvester

	if isTeamGame {
		winningTeam := computeWinningTeam(state.pendingRedScore, state.pendingBlueScore)
		if winningTeam == 0 || client.team != winningTeam {
			return false
		}
		// A team victory needs a positive winning score; when both teams
		// finish below zero (TDM bots falling off the map, say), nobody wins.
		winningScore := state.pendingRedScore
		if winningTeam == 2 {
			winningScore = state.pendingBlueScore
		}
		return winningScore != nil && *winningScore > 0
	}

	// FFA and Tournament: the top score wins only if it's above zero. Every
	// client starts at 0, so in a match nobody scored in (an instant rotation,
	// say) nobody wins.
	return hasFFAScores && maxFFAScore > 0 && client.score != nil && *client.score == maxFFAScore
}

// emitEvent delivers to the local channel and, when livePub is set
// (collector-only), tees onto the NATS live-event subject.
func (m *ServerManager) emitEvent(event domain.Event) {
	select {
	case m.events <- event:
	default:
		// The channel is full; drop the event.
	}
	if m.livePub != nil {
		if err := m.livePub.PublishLive(event); err != nil {
			log.Printf("collector: live publish %s failed: %v", event.Type, err)
		}
	}
}

// handleCommand dispatches a player's chat command.
func (m *ServerManager) handleCommand(ctx context.Context, serverID int64, state *serverState, clientID int, command string) {
	cmd, args := parseCommand(command)

	log.Printf("Command from client %d: cmd=%q args=%q", clientID, cmd, args)

	switch cmd {
	case "link":
		m.handleLinkCommand(ctx, serverID, state, clientID, args)
	case "claim":
		m.handleClaimCommand(ctx, serverID, state, clientID)
	case "help":
		m.handleHelpCommand(serverID, clientID)
	default:
		m.sendPrint(serverID, clientID, "^1Unknown command: ^7"+cmd+". Type ^3!help ^7for available commands.")
	}
}

func (m *ServerManager) handleHelpCommand(serverID int64, clientID int) {
	go func() {
		m.sendPrintSync(serverID, clientID, "^3Available commands:")
		m.sendPrintSync(serverID, clientID, "^3!claim^7: link this identity to an account")
		m.sendPrintSync(serverID, clientID, "^3!link <code>^7: link this identity to your account with a code from your Account page")
	}()
}

func (m *ServerManager) handleLinkCommand(ctx context.Context, serverID int64, state *serverState, clientID int, args string) {
	client, ok := state.clients[clientID]
	if !ok {
		log.Printf("link: client %d not found in state", clientID)
		return
	}

	code := trimSpace(args)
	if client.guid == "" {
		m.sendPrint(serverID, clientID, "^1Couldn't identify this player. Reconnect and try again.")
		return
	}

	reply, err := m.rpc.Link(ctx, hub.LinkRequest{GUID: client.guid, Code: code})
	if err != nil {
		log.Printf("link RPC error: %v", err)
		m.sendPrint(serverID, clientID, "^1Couldn't link this identity. Try again.")
		return
	}

	switch reply.Status {
	case hub.LinkOK:
		m.sendPrint(serverID, clientID, "^2This identity is now linked to your account.")
		log.Printf("Link successful: GUID %s via code %s", client.guid, code)
	case hub.LinkInvalidFormat:
		m.sendPrint(serverID, clientID, "^3Usage: ^7!link <6-digit-code>")
	case hub.LinkInvalidCode:
		m.sendPrint(serverID, clientID, "^1That link code is wrong or has expired.")
	case hub.LinkAlreadyLinked:
		m.sendPrint(serverID, clientID, "^3This identity is already linked to your account.")
	case hub.LinkUnknownGUID:
		m.sendPrint(serverID, clientID, "^1Couldn't find a player record for this identity.")
	case hub.LinkOtherAccount:
		m.sendPrint(serverID, clientID, "^1This identity is linked to another account.")
	default:
		log.Printf("link RPC error for GUID %s: %s", client.guid, reply.Message)
		m.sendPrint(serverID, clientID, "^1Couldn't link this identity. Ask for help in the Trinity Discord.")
	}
}

func (m *ServerManager) handleClaimCommand(ctx context.Context, serverID int64, state *serverState, clientID int) {
	client, ok := state.clients[clientID]
	if !ok {
		log.Printf("claim: client %d not found in state", clientID)
		return
	}

	if client.guid == "" {
		m.sendPrint(serverID, clientID, "^1Couldn't identify this player. Reconnect and try again.")
		return
	}

	reply, err := m.rpc.Claim(ctx, hub.ClaimRequest{GUID: client.guid})
	if err != nil {
		log.Printf("claim RPC error for GUID %s: %v", client.guid, err)
		m.sendPrint(serverID, clientID, "^1Couldn't create a claim code. Try again.")
		return
	}

	hubHost := "trinity.run"
	if m.cfg.Tracker != nil && m.cfg.Tracker.Collector != nil && m.cfg.Tracker.Collector.HubHost != "" {
		hubHost = m.cfg.Tracker.Collector.HubHost
	}
	switch reply.Status {
	case hub.ClaimOK:
		m.sendPrint(serverID, clientID, fmt.Sprintf("Your claim code is: ^3%s^7. Visit ^5%s ^7to claim this identity. Expires in 30 minutes.", reply.Code, hubHost))
		log.Printf("Claim code %s generated for GUID %s on server %d", reply.Code, client.guid, serverID)
	case hub.ClaimAlreadyClaimed:
		m.sendPrint(serverID, clientID, "^3This identity is already linked to an account.")
	case hub.ClaimUnknownPlayer:
		m.sendPrint(serverID, clientID, "^1Couldn't find your player record. Reconnect and try again.")
	default:
		log.Printf("claim RPC error: %s", reply.Message)
		m.sendPrint(serverID, clientID, "^1Couldn't create a claim code. Try again.")
	}
}

func (m *ServerManager) sendPrint(serverID int64, clientID int, message string) {
	go m.sendPrintSync(serverID, clientID, message)
}

// sendPrintSync is the synchronous form; use when ordering matters.
func (m *ServerManager) sendPrintSync(serverID int64, clientID int, message string) {
	cmd := fmt.Sprintf("sv_cmd print %d ^7%s\\n", clientID, message)
	log.Printf("Sending print to client %d: %q", clientID, message)
	if _, err := m.ExecuteRcon(serverID, cmd); err != nil {
		log.Printf("Error sending print to client %d on server %d: %v", clientID, serverID, err)
	}
}

func (m *ServerManager) sendCenterPrint(serverID int64, clientID int, message string) {
	// A literal \n survives the rcon transport.
	escaped := strings.ReplaceAll(message, "\n", "\\n")
	cmd := fmt.Sprintf("sv_cmd cp %d ^7%s", clientID, escaped)
	if _, err := m.ExecuteRcon(serverID, cmd); err != nil {
		log.Printf("Error sending cp to client %d on server %d: %v", clientID, serverID, err)
	}
}

// performGreet issues the greet RPC and sends the welcome. auth is nil when
// the handshake carried no login. When auth fails the player is still
// greeted, since the GUID already belongs to a known player, followed by a
// prominent notice to log in again.
func (m *ServerManager) performGreet(ctx context.Context, serverID int64, clientID int, guid, playerName, cleanName string, isVR, isTrinityEngine bool, auth *hub.AuthProof) {
	req := hub.GreetRequest{
		ServerID:   serverID,
		GUID:       guid,
		ClientName: playerName,
		CleanName:  cleanName,
		Auth:       auth,
	}
	reply, err := m.rpc.Greet(ctx, req)
	if err != nil {
		log.Printf("greet RPC error for GUID %s: %v", guid, err)
		return
	}

	if reply.AuthResult == hub.AuthFailed {
		m.sendTrinityAuthFail(serverID, clientID)
	} else if reply.AuthResult == hub.AuthVerified && reply.DisplayName != "" && !reply.NameConflict {
		userType := 1
		if reply.IsAdmin {
			userType = 2
		}
		m.sendTrinityAuthOk(serverID, clientID, userType, reply.DisplayName)
		playerName = reply.DisplayName
	}

	if reply.NameConflict {
		m.sendTrinityNameConflict(serverID, clientID)
	}

	// When the hub says the user may rcon this server, the encrypted password
	// goes out alongside the welcome.
	if reply.RconStuff != nil {
		m.sendRconAutoset(serverID, clientID, reply.RconStuff)
	}

	var message, cpMessage string
	hasStats := reply.CompletedMatches > 0
	switch {
	case reply.Claimed && hasStats:
		message = fmt.Sprintf("Welcome back, %s^7! K/D: ^3%.2f ^7| Matches: ^3%d ^7(^3!help ^7for commands)",
			playerName, reply.KDRatio, reply.CompletedMatches)
		cpMessage = fmt.Sprintf("Welcome back, %s^7!\nK/D: ^3%.2f ^7| Matches: ^3%d\n^3!help ^7for commands",
			playerName, reply.KDRatio, reply.CompletedMatches)
	case reply.Claimed:
		message = fmt.Sprintf("Welcome back, %s^7! (^3!help ^7for commands)", playerName)
		cpMessage = fmt.Sprintf("Welcome back, %s^7!\n^3!help ^7for commands", playerName)
	case hasStats:
		message = fmt.Sprintf("Welcome, %s^7! K/D: ^3%.2f ^7| Matches: ^3%d^7. Type ^3!claim ^7to link this identity to an account. (^3!help ^7for commands)",
			playerName, reply.KDRatio, reply.CompletedMatches)
		cpMessage = fmt.Sprintf("Welcome, %s^7!\nK/D: ^3%.2f ^7| Matches: ^3%d\nType ^3!claim ^7to link this identity to an account.",
			playerName, reply.KDRatio, reply.CompletedMatches)
	default:
		message = fmt.Sprintf("Welcome, %s^7! Type ^3!claim ^7to link this identity to an account. (^3!help ^7for commands)", playerName)
		cpMessage = fmt.Sprintf("Welcome, %s^7!\nType ^3!claim ^7to link this identity to an account.", playerName)
	}

	time.Sleep(3 * time.Second)
	m.sendPrintSync(serverID, clientID, message)
	m.sendCenterPrint(serverID, clientID, cpMessage)

	if reply.GUIDLinked {
		time.Sleep(2 * time.Second)
		m.sendPrint(serverID, clientID, "^2This identity is now linked to your account.")
	}

	if !isVR {
		var upgradeMsg, upgradeCpMsg string
		if strings.Contains(cleanName, "[VR]") {
			upgradeMsg = "Your VR client is out of date. Get the current Trinity at ^5trinity.run/docs ^7for every feature this server has."
			upgradeCpMsg = "Your VR client is out of date.\nGet the current Trinity at\n^5trinity.run/docs"
		} else if !isTrinityEngine {
			upgradeMsg = "This server has Trinity features your client can't show. Get Trinity at ^5trinity.run/docs^7."
			upgradeCpMsg = "This server runs Trinity.\nGet the client at\n^5trinity.run/docs"
		} else {
			upgradeMsg = "Haven't tried Quake III in VR yet? It's a whole new dimension (literally). See ^5trinity.run/docs^7."
			upgradeCpMsg = "Have a VR headset?\nTrinity plays in VR too.\n^5trinity.run/docs"
		}
		time.Sleep(3 * time.Second)
		m.sendPrintSync(serverID, clientID, upgradeMsg)
		m.sendCenterPrint(serverID, clientID, upgradeCpMsg)
	}

	if reply.AuthResult == hub.AuthFailed {
		time.Sleep(3 * time.Second)
		m.sendPrintSync(serverID, clientID, "^1Trinity login failed. ^7Your saved login is no longer valid. Log in again from Account on the main menu.")
		m.sendCenterPrint(serverID, clientID, "^1Login failed\n^7Log in again from the main menu")
	}
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// parseCommand splits a raw chat command into its verb and arguments. The mod
// appends ^7 to the logged command, so color codes are stripped before
// matching.
func parseCommand(command string) (cmd, args string) {
	command = domain.CleanQ3Name(command)
	cmd = command
	if idx := indexSpace(command); idx != -1 {
		cmd = command[:idx]
		args = trimSpace(command[idx+1:])
	}
	return cmd, args
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func indexSpace(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			return i
		}
	}
	return -1
}
