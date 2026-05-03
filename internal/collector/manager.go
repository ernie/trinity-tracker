package collector

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ernie/trinity-tracker/internal/config"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/hub"
)

// ServerManager orchestrates log parsing for all configured Q3 servers.
type ServerManager struct {
	cfg      *config.Config
	server   hub.ServerClient
	rpc      hub.RPCClient
	pub      hub.FactPublisher
	livePub  hub.LiveEventPublisher
	q3client *Q3Client
	events   chan domain.Event

	// replayCutoff overrides the per-server LastMatchEndedAt boundary
	// between replay and live events, so a collector restart in
	// distributed mode doesn't republish history already past the
	// hub's watermark.
	replayCutoff time.Time

	mu              sync.RWMutex
	servers         map[int64]*serverState
	tailers         map[int64]*LogTailer
	done            chan struct{}
	wg              sync.WaitGroup
	startupComplete bool
}

type serverState struct {
	server domain.Server
	match  *domain.Match
	// handshakeRequired tracks g_trinityHandshake for the current
	// match. When true, greeting waits for the TrinityHandshake event
	// (so any auth proof carried in the handshake can be applied to
	// the welcome), and match_start is refused for any server not
	// setting the cvar.
	handshakeRequired bool
	clients          map[int]*clientState    // client ID -> client state
	previousClients  map[string]*clientState // GUID -> accumulated stats from previous stints
	lastInitGame     time.Time               // dedupe InitGame and skip fake ShutdownGame at same timestamp
	matchState       string                  // "waiting", "warmup", "active", "overtime", "intermission"
	matchStarted     bool                    // true once MatchStart has been published (at WarmupEnd)
	matchFlushed     bool                    // true once match stats have been flushed
	warmupDuration   int                     // warmup duration in seconds (set when warmup starts)
	pendingExit      *string                 // exit reason from Exit event (deferred until scores captured)
	pendingExitAt    time.Time               // timestamp of Exit event
	pendingRedScore  *int                    // team scores captured at Exit time (before server resets)
	pendingBlueScore *int

	// GUIDs the collector knows have an open session on this server.
	// Mirrors the hub's sessions table for live, on-server players;
	// persists across InitGame so a map-change ClientBegin can be told
	// apart from a fresh connect (Q3 re-emits userinfo+Begin for every
	// persistent client at each map change). When set, the Begin is a
	// continuation: emit FactPresenceSnapshot, no greet. When unset,
	// it's a genuine join: emit FactPlayerJoin and greet.
	openSessions map[string]bool

	// Trinity handshake state
	trinityNonces    map[int]string           // map[clientNum]nonce
	pendingGreetings map[int]*pendingGreeting // map[clientNum]greeting awaiting handshake
}

// gauntletVictim tracks victim info for humiliation awards
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
	model              string // player model (e.g., "sarge/krusade")
	isBot              bool
	isVR               bool
	isTrinityEngine    bool
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

// SetLivePublisher enables teeing live events onto NATS. Used in
// collector-only mode; unset in hub+collector to avoid a self-loop.
func (m *ServerManager) SetLivePublisher(p hub.LiveEventPublisher) {
	m.livePub = p
}

// SetReplayCutoff pins the replay/live boundary for every tailed
// server. Must be called before Start. Zero uses each server's
// LastMatchEndedAt.
func (m *ServerManager) SetReplayCutoff(ts time.Time) {
	m.replayCutoff = ts.UTC()
}

// Events returns the event channel for WebSocket broadcasting
func (m *ServerManager) Events() <-chan domain.Event {
	return m.events
}

// Start initializes all servers and begins polling
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

		// Serial replay: concurrent tailers fight for the SQLite write lock.
		if srv.LogPath != "" {
			startAfter := m.cutoffFor(&srv, fullSrv)
			serverID := fullSrv.ID
			if !m.attachTailer(ctx, srv.Key, srv.LogPath, serverID, startAfter) {
				// Log file isn't there yet — common race when
				// trinity.service starts before quake3-server@.service
				// has had a chance to create the file. Poll for it in
				// the background; the tailer attaches as soon as it
				// shows up.
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

// freshLogThreshold separates "just-created" logs (replay everything as
// live) from "retrofit on a long-running q3 server" logs (skip history).
// /etc/logrotate.d/quake3 caps the active file at ~24h of activity, so
// any log over 10 MB is from a server that's been running long enough
// that we shouldn't backfill it on the hub.
const freshLogThreshold = 10 << 20 // 10 MB

// cutoffFor picks the replay cutoff for one server. Precedence:
//   1. Global m.replayCutoff (NATS publisher watermark) when set —
//      authoritative because it says "the hub already has up to this".
//   2. fullSrv.LastMatchEndedAt — hub-side per-server watermark.
//   3. File-size heuristic: small log = fresh, replay everything;
//      large log = retrofit, skip history (cutoff = now()).
func (m *ServerManager) cutoffFor(srvCfg *config.Q3Server, fullSrv *domain.Server) time.Time {
	if !m.replayCutoff.IsZero() {
		return m.replayCutoff
	}
	if fullSrv.LastMatchEndedAt != nil {
		return *fullSrv.LastMatchEndedAt
	}
	if info, err := os.Stat(srvCfg.LogPath); err == nil && info.Size() >= freshLogThreshold {
		return time.Now().UTC()
	}
	return time.Time{} // epoch — replay everything as live
}

// bootstrapServerPresence publishes a PresenceSnapshot for every
// currently-tracked, began client on one server. Called from
// attachTailer after replay completes — by then state.clients is
// populated. Idempotent on the hub: handlePresenceSnapshot overwrites
// the bySlot entry, no session work.
//
// Snapshots under the lock then publishes unlocked. Holding the lock
// across m.pub.Publish would block any concurrent writer (Stop, retry
// goroutines) on every NATS publish.
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
				GUID:         client.guid,
				Name:         client.name,
				CleanName:    client.cleanName,
				Model:        client.model,
				IsBot:        client.isBot,
				IsVR:         client.isVR,
				Skill:        client.skill,
				ClientNum:    client.clientID,
				Impressives:  client.impressives,
				Excellents:   client.excellents,
				Humiliations: client.humiliations,
				Defends:      client.defends,
				Captures:     client.captures,
				Assists:      client.assists,
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

// BootstrapAll republishes presence snapshots for every tracked
// server. Called after a NATS reconnect — when our connection drops
// and re-establishes, the hub on the other side has almost certainly
// just restarted (its embedded NATS goes down with it), and its
// in-memory presence map is empty. Re-bootstrapping rebuilds it
// without waiting for the next organic player_join.
//
// IDs are snapshotted under the lock then iterated unlocked, because
// bootstrapServerPresence acquires m.mu.RLock itself.
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

// Stop stops all polling and log watching
func (m *ServerManager) Stop() {
	log.Println("ServerManager: stopping...")
	close(m.done)
	// Snapshot under lock — tailWhenReady may write to m.tailers
	// concurrently and Go panics on concurrent map iteration + write.
	m.mu.RLock()
	tailers := make([]*LogTailer, 0, len(m.tailers))
	for _, t := range m.tailers {
		tailers = append(tailers, t)
	}
	m.mu.RUnlock()
	for _, tailer := range tailers {
		tailer.Stop()
	}
	m.wg.Wait()
	log.Println("ServerManager: shutdown complete")
}

// attachTailer opens the log file, replays from startAfter, publishes
// a presence bootstrap snapshot for everything that ended up in
// state.clients, and starts the live tail. Returns false if the log
// file can't be opened (caller can decide to schedule a retry); returns
// true after the tail goroutine is running. Errors past the OpenFile
// stage are logged and swallowed — they aren't grounds for tearing
// down the manager.
//
// Bootstrap order is *after* replay but *before* spawning the live
// processLogEvents goroutine. That places the snapshot ahead of any
// new live events on the wire, so the hub sees a consistent slot
// state before live updates start arriving.
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
	// If Stop() ran in the window between Start() above and this lock,
	// m.done is already closed and Stop()'s tailer snapshot has missed
	// us. Bail and stop the tailer ourselves.
	select {
	case <-m.done:
		m.mu.Unlock()
		tailer.Stop()
		return true
	default:
	}
	m.tailers[serverID] = tailer
	m.mu.Unlock()
	m.wg.Add(1)
	go m.processLogEvents(ctx, serverID, tailer)
	return true
}

// tailWhenReady polls for the log file and attaches the tailer as soon
// as the q3 server creates it. Most operators see this fire only on
// fresh installs where trinity.service starts before quake3-server@
// has written its first log line.
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

// GetServerStatus returns the current status for a server
// ExecuteRcon sends an RCON command to a server and returns the response
func (m *ServerManager) ExecuteRcon(serverID int64, command string) (string, error) {
	m.mu.RLock()
	state, ok := m.servers[serverID]
	m.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("server not found")
	}

	// Find RCON password from config
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

// HasRconAccess checks if a server has RCON configured
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

// ExecuteRconByKey runs an RCON command against the server identified
// by its per-collector key (not the hub's global server ID). Used by
// the NATS RCON proxy handler — the hub forwards (key, command) and
// the collector resolves locally.
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

// AdminDelegationFor returns whether the operator opted this server
// (by key) in to hub-admin RCON delegation. Match is case-insensitive
// on key. Used by the collector-side RCON handler to re-validate hub
// admin requests against current config.
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

// Roster returns the current list of registered servers as RegdServer
// entries. Used by the distributed-tracking Registrar to broadcast the
// collector's roster on heartbeat. AdminDelegationEnabled mirrors the
// per-server config flag so the hub UI can decide whether a hub admin
// gets the click-to-RCON affordance.
func (m *ServerManager) Roster() []domain.RegdServer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Resolve cfg flag by address (cfg.Q3Servers and m.servers both key
	// off Address; see ExecuteRcon below for the same lookup pattern).
	delegationByAddress := make(map[string]bool, len(m.cfg.Q3Servers))
	for _, srv := range m.cfg.Q3Servers {
		delegationByAddress[srv.Address] = srv.AllowHubAdminRcon
	}
	out := make([]domain.RegdServer, 0, len(m.servers))
	for _, state := range m.servers {
		out = append(out, domain.RegdServer{
			LocalID:                state.server.ID,
			Key:                    state.server.Key,
			Address:                state.server.Address,
			AdminDelegationEnabled: delegationByAddress[state.server.Address],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LocalID < out[j].LocalID })
	return out
}

// processLogEvents handles events from a log tailer
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

// handleLogEvent processes a single log event.
// When replayMode is true, only in-memory state is updated (no DB writes, no event emission).
// This is used during startup to rebuild client state from already-processed log entries.
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
		m.handleMatchChange(ctx, state, data.MapName, data.GameType, data.UUID, data.Settings["g_movement"], data.Settings["g_gameplay"], data.Settings["g_trinityhandshake"] == "1", event.Timestamp, replayMode)

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
					// Publish match_start regardless of g_trinityHandshake so
					// the hub's handshake_required column tracks the cvar in
					// both directions. The hub still gates persistence on
					// HandshakeRequired, so non-handshake matches don't
					// produce stats — but the server's UI visibility flips
					// off without manual intervention. matchStarted gates
					// match_end below; we only set it when the hub created
					// a match row, so we don't fire match_end against a
					// UUID the hub never persisted.
					m.pub.Publish(domain.FactEvent{
						Type:      domain.FactMatchStart,
						ServerID:  state.server.ID,
						Timestamp: event.Timestamp,
						Data: domain.MatchStartData{
							MatchUUID:         state.match.UUID,
							MapName:           state.match.MapName,
							GameType:          state.match.GameType,
							Movement:          state.match.Movement,
							Gameplay:          state.match.Gameplay,
							StartedAt:         event.Timestamp,
							HandshakeRequired: state.handshakeRequired,
						},
					})
					if state.handshakeRequired {
						state.matchStarted = true
					} else {
						log.Printf("collector: match_start for %s on server %s/%s with g_trinityHandshake not enabled — hub will reject stats and downgrade the server", state.match.UUID, state.server.Source, state.server.Key)
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

		// Intermission = match is over. Flush all stats and end match.
		// matchStarted gate matches the shutdown path: if match_start was
		// never published (e.g. non-handshake server), the hub has no
		// match row to attach match_end to. Skip in replay mode — any
		// match_end for a pre-watermark match was already sent on its
		// original run; republishing would flood the hub.
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
		wasBegan := client.began
		client.name = data.Name
		client.cleanName = domain.CleanQ3Name(data.Name)
		client.isBot = data.IsBot
		client.isVR = data.IsVR
		client.isTrinityEngine = data.IsTrinityEngine
		client.skill = data.Skill
		client.team = data.Team
		client.model = data.Model

		// Canonical GUID: bots use synthetic "BOT:<cleanName>"; humans use
		// the literal GUID from the log. Empty until identity is known.
		if data.IsBot {
			client.guid = "BOT:" + client.cleanName
		} else {
			client.guid = data.GUID
		}

		// If the client is already in the game and a userinfo field
		// the hub tracks changed, forward the change so the live card
		// reflects the new portrait / VR flag without faking a rejoin.
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

		// Resolve identity through the hub writer. In replay mode we
		// skip the RPC entirely — the return value isn't used (the
		// hub-side presence tracker answers PlayerID / verified /
		// admin now), and blasting hundreds of bot+human upserts at
		// the hub during log replay dominates collector startup time.
		// New identities introduced post-watermark hit the live path
		// below. The edge case (bot GUID that only appeared
		// pre-watermark and isn't in the hub DB yet) results in its
		// match_end stats being dropped hub-side with a log message;
		// acceptable since that bot will be re-upserted the next time
		// it joins.
		if client.guid != "" && !replayMode {
			var err error
			if data.IsBot {
				_, err = m.server.UpsertBotPlayerIdentity(ctx, data.Name, client.cleanName, event.Timestamp)
			} else {
				_, err = m.server.UpsertPlayerIdentity(ctx, data.GUID, data.Name, client.cleanName, event.Timestamp, data.IsVR)
			}
			if err != nil {
				log.Printf("Error resolving player identity for GUID %s: %v", client.guid, err)
			}
		}

	case EventTypeClientBegin:
		data := event.Data.(ClientConnectData)
		if client, ok := state.clients[data.ClientID]; ok {
			client.began = true

			// A Begin is either a genuine fresh join or a map-change
			// continuation. The collector mirrors the hub's session
			// table in state.openSessions so it can tell them apart
			// without round-tripping; the publish type and the greet
			// both branch off the same answer.
			if client.guid != "" && !replayMode {
				if state.openSessions[client.guid] {
					m.pub.Publish(domain.FactEvent{
						Type:      domain.FactPresenceSnapshot,
						ServerID:  serverID,
						Timestamp: event.Timestamp,
						Data: domain.PresenceSnapshotData{
							GUID:      client.guid,
							Name:      client.name,
							CleanName: client.cleanName,
							Model:     client.model,
							IsBot:     client.isBot,
							IsVR:      client.isVR,
							Skill:     client.skill,
							ClientNum: data.ClientID,
						},
					})
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
					// Greet only humans; bots' synthetic GUID passes
					// guid != "" but they shouldn't get a welcome.
					if !client.isBot && m.startupComplete {
						if m.handshakeRequired(state) {
							// Delay greeting until handshake completes (or warn on timeout).
							// The handshake handler calls performGreet with auth bundled in.
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

			// Preserve stats for match-end flush (unless match already flushed)
			// Skip clients that never began (connected but never spawned)
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

		// Only track frags/deaths during active gameplay (not warmup/waiting/intermission)
		// Note: We track stats even during replay so we can flush them if match wasn't completed
		if state.matchState == "active" || state.matchState == "overtime" {
			// Increment in-memory frag count for fragger (human or bot)
			if fragger, ok := state.clients[data.FraggerID]; ok {
				fragger.frags++
			}

			// Increment in-memory death count for victim (human or bot)
			if victim, ok := state.clients[data.VictimID]; ok {
				victim.deaths++
			}

			// Track gauntlet frag victim for humiliation award (MOD_GAUNTLET = 2)
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
		// Capture score and team at match end (score events fire during Exit sequence)
		data := event.Data.(ScoreEventData)
		if client, ok := state.clients[data.ClientID]; ok {
			score := data.Score
			client.score = &score
			client.team = data.Team
		}

	case EventTypeExit:
		data := event.Data.(ExitEventData)
		if state.match != nil {
			// Defer stats flush until ShutdownGame to capture score events that follow Exit
			// Score events are emitted AFTER Exit but BEFORE ShutdownGame
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
				// already flushed or pre-watermark replay; hub dedup
				// via source_progress.consumed_seq handles any
				// boundary duplicates.
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
				// Match never reached active play (no WarmupEnd) - discard without persisting
				log.Printf("Discarding match %s: never reached active play", state.match.UUID)
			}
		}
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
		// Track capture in memory for real-time display
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
		// Skip events in replay mode
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
		// Track flag return in memory for stats (only player-initiated returns, not auto-returns)
		if data.ClientID >= 0 {
			if client, ok := state.clients[data.ClientID]; ok {
				client.flagReturns++
			}
		}
		if !replayMode {
			var guid string
			// Auto-returns have ClientID == -1 and no player associated
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
		// Skip events in replay mode
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
		// Skip events in replay mode
		if !replayMode {
			var guid string
			if client, ok := state.clients[data.AttackerID]; ok {
				guid = client.guid
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

	case EventTypeSkullScore:
		data := event.Data.(SkullScoreData)
		// Skip events in replay mode
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

			// When leaving a playing team, preserve stats for match-end flush
			if oldTeam != 3 && oldTeam != data.NewTeam && client.guid != "" {
				if state.matchStarted &&
					(state.matchState == "active" || state.matchState == "overtime") {
					state.savePreviousClient(client)

					// Create fresh clientState for new team, carrying forward identity
					initialScore := 0
					state.clients[data.ClientID] = &clientState{
						clientID:  client.clientID,
						name:      client.name,
						cleanName: client.cleanName,
						guid:      client.guid,
						model:     client.model,
						isBot:     client.isBot,
						isVR:      client.isVR,
						skill:     client.skill,
						team:      data.NewTeam,
						joinedAt:  event.Timestamp,
						ipAddress: client.ipAddress,
						began:     client.began,
						score:     &initialScore,
					}
				} else {
					// No active match — just update team
					client.team = data.NewTeam
				}
			} else {
				client.team = data.NewTeam
				// Update joinedAt when entering a playing team from spectator
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
		data := event.Data.(AssistData)
		if client, ok := state.clients[data.ClientID]; ok {
			client.assists++
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
			case "assist":
				client.assists++
			}

			if !replayMode {
				// Map gauntlet to humiliation for frontend
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

				// Include victim info for humiliation awards
				if data.AwardType == "gauntlet" && client.lastGauntletVictim != nil {
					awardEvent.VictimName = client.lastGauntletVictim.name
					awardEvent.VictimGUID = client.lastGauntletVictim.guid
					// Clear victim after using it
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
		// Only process commands after startup is complete (skip during log replay)
		if !replayMode && m.startupComplete {
			m.handleCommand(ctx, serverID, state, data.ClientID, data.Command)
		}

	case EventTypeSay:
		data := event.Data.(SayData)
		// Skip events in replay mode
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
		// Skip events in replay mode
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
		// Skip events in replay mode
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
		// Skip events in replay mode
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
		// No fact emitted — absence of FactDemoFinalized for the match
		// is the same signal hub-side. Logged for replay/debug.

	case EventTypeCvarChange:
		data := event.Data.(CvarChangeData)
		if state.match != nil && state.match.UUID != "" {
			switch data.Key {
			case "g_movement":
				state.match.Movement = data.Value
				m.pub.Publish(domain.FactEvent{
					Type:      domain.FactMatchSettingsUpdate,
					ServerID:  state.server.ID,
					Timestamp: event.Timestamp,
					Data:      domain.MatchSettingsUpdateData{MatchUUID: state.match.UUID, Movement: data.Value},
				})
			case "g_gameplay":
				state.match.Gameplay = data.Value
				m.pub.Publish(domain.FactEvent{
					Type:      domain.FactMatchSettingsUpdate,
					ServerID:  state.server.ID,
					Timestamp: event.Timestamp,
					Data:      domain.MatchSettingsUpdateData{MatchUUID: state.match.UUID, Gameplay: data.Value},
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

// handleMapChange handles a new map starting
func (m *ServerManager) handleMatchChange(ctx context.Context, state *serverState, mapName string, gameType int, uuid string, movement string, gameplay string, handshakeEnabled bool, ts time.Time, replayMode bool) {
	state.handshakeRequired = handshakeEnabled
	// Skip duplicate InitGame at same timestamp (Q3 sometimes logs it twice on server restart)
	if ts.Equal(state.lastInitGame) {
		return
	}
	state.lastInitGame = ts

	gameTypeStr := domain.GameTypeFromInt(gameType)

	// Replay just rebuilds in-memory state; duplicate publishes are
	// deduped on the hub via consumed_seq / the publish watermark.
	if replayMode {
		prevMatch := state.match

		// Emit match_crashed for a prior match that started but never
		// ended. Hub-side handler is idempotent.
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
			Movement:  movement,
			Gameplay:  gameplay,
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

	// Defer DB persistence until WarmupEnd so warmup-only matches
	// don't create orphaned rows.
	match := &domain.Match{
		UUID:      uuid,
		ServerID:  state.server.ID,
		MapName:   mapName,
		GameType:  gameTypeStr,
		StartedAt: ts,
		Movement:  movement,
		Gameplay:  gameplay,
	}
	state.match = match
	state.matchStarted = false

	state.clients = make(map[int]*clientState)
	state.previousClients = make(map[string]*clientState)
	state.matchFlushed = false
	state.matchState = "" // will be set by MatchState event if warmup enabled
	state.warmupDuration = 0

	// Emit match start event
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
		prev.clientID = client.clientID
		prev.team = client.team
		prev.model = client.model
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
			GUID:         client.guid,
			ClientID:     client.clientID,
			Name:         client.name,
			CleanName:    client.cleanName,
			Frags:        client.frags,
			Deaths:       client.deaths,
			Completed:    false,
			Score:        client.score,
			Team:         team,
			Model:        client.model,
			Skill:        client.skill,
			Victory:      false,
			Captures:     client.captures,
			FlagReturns:  client.flagReturns,
			Assists:      client.assists,
			Impressives:  client.impressives,
			Excellents:   client.excellents,
			Humiliations: client.humiliations,
			Defends:      client.defends,
			IsBot:        client.isBot,
			JoinedLate:   joinedLate,
			JoinedAt:     client.joinedAt,
			IsVR:         client.isVR,
		})
	}

	// Connected players last (completed=true)
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
			GUID:         client.guid,
			ClientID:     clientID,
			Name:         client.name,
			CleanName:    client.cleanName,
			Frags:        client.frags,
			Deaths:       client.deaths,
			Completed:    true,
			Score:        client.score,
			Team:         team,
			Model:        client.model,
			Skill:        client.skill,
			Victory:      victory,
			Captures:     client.captures,
			FlagReturns:  client.flagReturns,
			Assists:      client.assists,
			Impressives:  client.impressives,
			Excellents:   client.excellents,
			Humiliations: client.humiliations,
			Defends:      client.defends,
			IsBot:        client.isBot,
			JoinedLate:   joinedLate,
			JoinedAt:     client.joinedAt,
			IsVR:         client.isVR,
		})
	}

	return players
}

// computeWinningTeam returns the winning team (1=red, 2=blue) or 0 for tie/unknown
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

// computeMaxScore returns the maximum score among all completed clients (for FFA victory)
// Returns (maxScore, found) where found indicates at least one eligible client existed
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

// isMatchWinner determines if a client won the match based on game type and scores
func isMatchWinner(client *clientState, state *serverState, maxFFAScore int, hasFFAScores bool) bool {
	if state.match == nil || state.pendingExit == nil {
		return false // No victory for abnormal shutdown
	}

	gameType := state.match.GameType
	isTeamGame := gameType == "ctf" || gameType == "tdm" || gameType == "team" ||
		gameType == "obelisk" || gameType == "harvester" || gameType == "1fctf"

	if isTeamGame {
		winningTeam := computeWinningTeam(state.pendingRedScore, state.pendingBlueScore)
		return winningTeam > 0 && client.team == winningTeam
	}

	// FFA/Tournament: highest score wins
	return hasFFAScores && client.score != nil && *client.score == maxFFAScore
}

// emitEvent delivers to the local channel and, if livePub is set,
// tees onto the NATS live-event subject (collector-only deployments).
func (m *ServerManager) emitEvent(event domain.Event) {
	select {
	case m.events <- event:
	default:
		// Channel full, drop event
	}
	if m.livePub != nil {
		if err := m.livePub.PublishLive(event); err != nil {
			log.Printf("collector: live publish %s failed: %v", event.Type, err)
		}
	}
}

// handleCommand dispatches a command to the appropriate handler
func (m *ServerManager) handleCommand(ctx context.Context, serverID int64, state *serverState, clientID int, command string) {
	// Parse command name and args: "link 12345678" -> cmd="link", args="12345678"
	cmd := command
	args := ""
	if idx := indexSpace(command); idx != -1 {
		cmd = command[:idx]
		args = trimSpace(command[idx+1:])
	}

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

// handleHelpCommand shows available commands to a player
func (m *ServerManager) handleHelpCommand(serverID int64, clientID int) {
	go func() {
		m.sendPrintSync(serverID, clientID, "^3Available commands:")
		m.sendPrintSync(serverID, clientID, "^3!claim ^7- Link your identity to an account")
		m.sendPrintSync(serverID, clientID, "^3!link <code> ^7- Link current identity to your account")
	}()
}

// handleLinkCommand processes a link command from a player
func (m *ServerManager) handleLinkCommand(ctx context.Context, serverID int64, state *serverState, clientID int, args string) {
	client, ok := state.clients[clientID]
	if !ok {
		log.Printf("link: client %d not found in state", clientID)
		return
	}

	code := trimSpace(args)
	if client.guid == "" {
		m.sendPrint(serverID, clientID, "^1Error: Current identity unknown. Try reconnecting.")
		return
	}

	reply, err := m.rpc.Link(ctx, hub.LinkRequest{GUID: client.guid, Code: code})
	if err != nil {
		log.Printf("link RPC error: %v", err)
		m.sendPrint(serverID, clientID, "^1Error linking account. Please try again.")
		return
	}

	switch reply.Status {
	case hub.LinkOK:
		m.sendPrint(serverID, clientID, "^2Link successful! ^7This identity has been linked to your account.")
		log.Printf("Link successful: GUID %s via code %s", client.guid, code)
	case hub.LinkInvalidFormat:
		m.sendPrint(serverID, clientID, "^3Usage: ^7!link <6-digit-code>")
	case hub.LinkInvalidCode:
		m.sendPrint(serverID, clientID, "^1Invalid or expired link code.")
	case hub.LinkAlreadyLinked:
		m.sendPrint(serverID, clientID, "^3This identity is already linked to your account.")
	case hub.LinkUnknownGUID:
		m.sendPrint(serverID, clientID, "^1Error: Could not find player record for this identity.")
	default:
		log.Printf("link RPC error for GUID %s: %s", client.guid, reply.Message)
		m.sendPrint(serverID, clientID, "^1Error linking account. Please contact admin.")
	}
}

// handleClaimCommand processes a claim command from a player
func (m *ServerManager) handleClaimCommand(ctx context.Context, serverID int64, state *serverState, clientID int) {
	client, ok := state.clients[clientID]
	if !ok {
		log.Printf("claim: client %d not found in state", clientID)
		return
	}

	if client.guid == "" {
		m.sendPrint(serverID, clientID, "^1Error: Current identity unknown. Try reconnecting.")
		return
	}

	reply, err := m.rpc.Claim(ctx, hub.ClaimRequest{GUID: client.guid})
	if err != nil {
		log.Printf("claim RPC error for GUID %s: %v", client.guid, err)
		m.sendPrint(serverID, clientID, "^1Error generating claim code. Please try again.")
		return
	}

	hubHost := "trinity.run"
	if m.cfg.Tracker != nil && m.cfg.Tracker.Collector != nil && m.cfg.Tracker.Collector.HubHost != "" {
		hubHost = m.cfg.Tracker.Collector.HubHost
	}
	switch reply.Status {
	case hub.ClaimOK:
		m.sendPrint(serverID, clientID, fmt.Sprintf("Your claim code is: ^3%s^7 - Visit ^5%s ^7to claim this identity. Expires in 30 minutes.", reply.Code, hubHost))
		log.Printf("Claim code %s generated for GUID %s on server %d", reply.Code, client.guid, serverID)
	case hub.ClaimAlreadyClaimed:
		m.sendPrint(serverID, clientID, "^3This identity is already linked to an account.")
	case hub.ClaimUnknownPlayer:
		m.sendPrint(serverID, clientID, "^1Error: Could not identify your player record. Try reconnecting.")
	default:
		log.Printf("claim RPC error: %s", reply.Message)
		m.sendPrint(serverID, clientID, "^1Error generating claim code. Please try again.")
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
	// Literal \n so RCON transport doesn't eat the newlines.
	escaped := strings.ReplaceAll(message, "\n", "\\n")
	cmd := fmt.Sprintf("sv_cmd cp %d ^7%s", clientID, escaped)
	if _, err := m.ExecuteRcon(serverID, cmd); err != nil {
		log.Printf("Error sending cp to client %d on server %d: %v", clientID, serverID, err)
	}
}

// performGreet issues the greet RPC and applies the reply with a
// formatted welcome. auth is nil when the client sent no Trinity
// handshake auth info. On AuthFailed we still greet — the GUID
// already binds to a known player and can't be rebound — but follow
// up with a prominent re-login notice.
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
		// Clears cl_trinityToken/cl_trinityUser engine-side.
		m.sendTrinityAuthFail(serverID, clientID)
	}

	var message, cpMessage string
	hasStats := reply.CompletedMatches > 0
	switch {
	case reply.Claimed && hasStats:
		message = fmt.Sprintf("Welcome back, %s^7! K/D: ^3%.2f ^7| Matches: ^3%d ^7(^3!help ^7for help)",
			playerName, reply.KDRatio, reply.CompletedMatches)
		cpMessage = fmt.Sprintf("Welcome back, %s^7!\nK/D: ^3%.2f ^7| Matches: ^3%d\n^3!help ^7for help",
			playerName, reply.KDRatio, reply.CompletedMatches)
	case reply.Claimed:
		message = fmt.Sprintf("Welcome back, %s^7! (^3!help ^7for help)", playerName)
		cpMessage = fmt.Sprintf("Welcome back, %s^7!\n^3!help ^7for help", playerName)
	case hasStats:
		message = fmt.Sprintf("Welcome, %s^7! K/D: ^3%.2f ^7| Matches: ^3%d ^7- ^3!claim ^7to link your identity! (^3!help ^7for help)",
			playerName, reply.KDRatio, reply.CompletedMatches)
		cpMessage = fmt.Sprintf("Welcome, %s^7!\nK/D: ^3%.2f ^7| Matches: ^3%d\n^3!claim ^7to link your identity!",
			playerName, reply.KDRatio, reply.CompletedMatches)
	default:
		message = fmt.Sprintf("Welcome, %s^7! ^3!claim ^7to link your identity! (^3!help ^7for help)", playerName)
		cpMessage = fmt.Sprintf("Welcome, %s^7!\n^3!claim ^7to link your identity!", playerName)
	}

	time.Sleep(3 * time.Second)
	m.sendPrintSync(serverID, clientID, message)
	m.sendCenterPrint(serverID, clientID, cpMessage)

	if reply.GUIDLinked {
		time.Sleep(2 * time.Second)
		m.sendPrint(serverID, clientID, "^2This identity has been linked to your account.")
	}

	if !isVR {
		var upgradeMsg, upgradeCpMsg string
		if strings.Contains(cleanName, "[VR]") {
			upgradeMsg = "Your VR client is outdated. Upgrade to enjoy all Trinity features! More info at ^5trinity.run/docs"
			upgradeCpMsg = "Your VR client is outdated.\nUpgrade to enjoy all Trinity features!\n^5trinity.run/docs"
		} else if !isTrinityEngine {
			upgradeMsg = "It looks like you're missing out on Trinity-specific features on this server. Go to ^5trinity.run/docs ^7to upgrade."
			upgradeCpMsg = "Get Trinity for the best experience!\n^5trinity.run/docs"
		} else {
			upgradeMsg = "Haven't tried Quake 3 in VR yet? It's a whole new dimension (literally). Visit ^5trinity.run/docs ^7to learn more."
			upgradeCpMsg = "Do you play VR?\nGet the VR client!\n^5trinity.run/docs"
		}
		time.Sleep(3 * time.Second)
		m.sendPrintSync(serverID, clientID, upgradeMsg)
		m.sendCenterPrint(serverID, clientID, upgradeCpMsg)
	}

	if reply.AuthResult == hub.AuthFailed {
		time.Sleep(3 * time.Second)
		m.sendPrintSync(serverID, clientID, "^1Trinity authentication failed. ^7Your saved credentials are invalid — log in again from the main menu to verify your identity.")
		m.sendCenterPrint(serverID, clientID, "^1Authentication failed\n^7Log in again from the main menu")
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

