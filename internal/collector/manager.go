package collector

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ernie/trinity-tracker/internal/config"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/hub"
)

// ServerManager orchestrates polling and log parsing for all servers
type ServerManager struct {
	cfg      *config.Config
	writer   *hub.Writer
	rpc      hub.RPCClient // greet/claim/link entry point; falls back to writer in standalone mode
	q3client *Q3Client
	events   chan domain.Event

	// replayCutoff, when non-zero, overrides the per-server
	// LastMatchEndedAt as the boundary between replayMode events (no
	// DB writes, no publishes) and live events. Distributed mode sets
	// this from the NATS publish watermark so we never re-publish
	// events that already reached the hub.
	replayCutoff time.Time

	mu              sync.RWMutex
	servers         map[int64]*serverState
	tailers         map[int64]*LogTailer
	done            chan struct{}
	wg              sync.WaitGroup // track goroutine completion for graceful shutdown
	startupComplete bool           // true after Start() finishes, enables !link command processing
}

// serverState tracks the current state of a monitored server
type serverState struct {
	server           domain.Server
	status           *domain.ServerStatus
	match            *domain.Match
	clients          map[int]*clientState      // client ID -> client state
	previousClients  map[string]*clientState    // GUID -> accumulated stats from previous stints
	lastInitGame     time.Time                  // dedupe InitGame and skip fake ShutdownGame at same timestamp
	matchState       string                     // "waiting", "warmup", "active", "overtime", "intermission"
	matchStarted     bool                       // true once MatchStart has been published (at WarmupEnd)
	matchFlushed     bool                       // true once match stats have been flushed
	warmupDuration   int                        // warmup duration in seconds (set when warmup starts)
	pendingExit      *string                    // exit reason from Exit event (deferred until scores captured)
	pendingExitAt    time.Time                  // timestamp of Exit event
	pendingRedScore  *int                       // team scores captured at Exit time (before server resets)
	pendingBlueScore *int
	// Trinity handshake state
	trinityNonces    map[int]string           // map[clientNum]nonce
	pendingGreetings map[int]*pendingGreeting // map[clientNum]greeting awaiting handshake
}

// gauntletVictim tracks victim info for humiliation awards
type gauntletVictim struct {
	name string
	guid string
}

// clientState tracks a connected client
type clientState struct {
	clientID           int
	playerID           int64 // ID of the players record (cached from writer)
	isVerified         bool  // player has a linked user account
	isAdmin            bool  // player's linked user is an admin
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

// getPlayerIDPtr returns a pointer to the player ID if valid, nil otherwise
func (c *clientState) getPlayerIDPtr() *int64 {
	if c.playerID > 0 {
		return &c.playerID
	}
	return nil
}

// NewServerManager creates a new manager. rpc is the greet/claim/link
// entry point; pass writer in standalone mode (it satisfies
// hub.RPCClient), or a natsbus RPC client in distributed mode.
func NewServerManager(cfg *config.Config, writer *hub.Writer, rpc hub.RPCClient) *ServerManager {
	if rpc == nil {
		rpc = writer
	}
	return &ServerManager{
		cfg:      cfg,
		writer:   writer,
		rpc:      rpc,
		q3client: NewQ3Client(),
		events:   make(chan domain.Event, 100),
		servers:  make(map[int64]*serverState),
		tailers:  make(map[int64]*LogTailer),
		done:     make(chan struct{}),
	}
}

// SetReplayCutoff pins the boundary between replayMode and live
// events across every server this manager tails. Must be called
// before Start so the first replay pass honors it. Zero means "use
// each server's LastMatchEndedAt" (standalone behavior).
func (m *ServerManager) SetReplayCutoff(ts time.Time) {
	m.replayCutoff = ts.UTC()
}

// Events returns the event channel for WebSocket broadcasting
func (m *ServerManager) Events() <-chan domain.Event {
	return m.events
}

// Start initializes all servers and begins polling
func (m *ServerManager) Start(ctx context.Context) error {
	// Register servers through the hub writer and replay logs synchronously
	for _, srv := range m.cfg.Q3Servers {
		fullSrv, err := m.writer.RegisterServer(ctx, srv.Name, srv.Address, srv.LogPath)
		if err != nil {
			return err
		}

		m.servers[fullSrv.ID] = &serverState{
			server:        *fullSrv,
			clients:       make(map[int]*clientState),
			trinityNonces: make(map[int]string),
		}

		// Replay log events synchronously (one server at a time to avoid DB lock contention)
		if srv.LogPath != "" {
			startAfter := time.Time{} // epoch - replay all
			if !m.replayCutoff.IsZero() {
				startAfter = m.replayCutoff
			} else if fullSrv.LastMatchEndedAt != nil {
				startAfter = *fullSrv.LastMatchEndedAt
			}

			tailer := NewLogTailer(srv.LogPath, nil)
			if _, err := tailer.OpenFile(); err != nil {
				log.Printf("Warning: failed to open log file for %s: %v", srv.Name, err)
				continue
			}

			log.Printf("Replaying log for %s from %v", srv.Name, startAfter)
			serverID := fullSrv.ID
			if err := tailer.ReplayFromTimestamp(startAfter, func(event LogEvent, replayMode bool) {
				m.handleLogEvent(ctx, serverID, event, replayMode)
			}); err != nil {
				log.Printf("Warning: failed to replay log for %s: %v", srv.Name, err)
			}

			// Now start tailing for new events
			if err := tailer.Start(); err != nil {
				log.Printf("Warning: failed to start log tailer for %s: %v", srv.Name, err)
				tailer.Stop()
			} else {
				m.tailers[fullSrv.ID] = tailer
				m.wg.Add(1)
				go m.processLogEvents(ctx, fullSrv.ID, tailer)
			}
		}
	}

	// Start UDP polling
	m.wg.Add(1)
	go m.pollLoop(ctx)

	// Mark startup complete - enables !link command processing
	m.mu.Lock()
	m.startupComplete = true
	m.mu.Unlock()
	log.Printf("Startup complete, !link commands now enabled")

	return nil
}

// Stop stops all polling and log watching
func (m *ServerManager) Stop() {
	log.Println("ServerManager: stopping...")
	close(m.done)
	for _, tailer := range m.tailers {
		tailer.Stop()
	}
	m.wg.Wait()
	log.Println("ServerManager: shutdown complete")
}

// GetServerStatus returns the current status for a server
func (m *ServerManager) GetServerStatus(serverID int64) *domain.ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, ok := m.servers[serverID]; ok {
		return state.status
	}
	return nil
}

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

// GetAllStatuses returns current status for all servers
// Roster returns the current list of registered servers as RegdServer
// entries. Used by the distributed-tracking Registrar to broadcast the
// collector's roster on heartbeat.
func (m *ServerManager) Roster() []domain.RegdServer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.RegdServer, 0, len(m.servers))
	for _, state := range m.servers {
		out = append(out, domain.RegdServer{
			LocalID: state.server.ID,
			Name:    state.server.Name,
			Address: state.server.Address,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LocalID < out[j].LocalID })
	return out
}

func (m *ServerManager) GetAllStatuses() []domain.ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var statuses []domain.ServerStatus
	for _, state := range m.servers {
		if state.status != nil {
			statuses = append(statuses, *state.status)
		}
	}

	// Sort by server ID for consistent ordering
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].ServerID < statuses[j].ServerID
	})

	return statuses
}

// pollLoop periodically queries all servers via UDP
func (m *ServerManager) pollLoop(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(m.cfg.Server.PollInterval)
	defer ticker.Stop()

	// Initial poll
	m.pollAll(ctx)

	for {
		select {
		case <-m.done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.pollAll(ctx)
		}
	}
}

// pollAll queries all servers
func (m *ServerManager) pollAll(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for serverID, state := range m.servers {
		status, err := m.q3client.QueryStatus(state.server.Address)
		if err != nil {
			log.Printf("Error polling %s: %v", state.server.Name, err)

			// Create or update offline status
			if state.status == nil {
				state.status = &domain.ServerStatus{
					ServerID: serverID,
					Name:     state.server.Name,
					Address:  state.server.Address,
				}
			}
			state.status.Online = false
			state.status.LastUpdated = time.Now().UTC()

			// Emit offline status to notify clients
			m.emitEvent(domain.Event{
				Type:      domain.EventServerUpdate,
				ServerID:  serverID,
				Timestamp: time.Now().UTC(),
				Data:      state.status,
			})
			continue
		}

		status.ServerID = serverID
		status.Name = state.server.Name

		// Calculate game time from match start
		if state.match != nil {
			status.GameTimeMs = int(time.Since(state.match.StartedAt).Milliseconds())
		}

		// Enrich UDP players with bot status from tracked clients
		m.enrichPlayersFromClients(state, status)

		state.status = status

		// Emit server update event
		m.emitEvent(domain.Event{
			Type:      domain.EventServerUpdate,
			ServerID:  serverID,
			Timestamp: time.Now().UTC(),
			Data:      status,
		})
	}
}

// enrichPlayersFromClients sets IsBot and other fields based on tracked client data
func (m *ServerManager) enrichPlayersFromClients(state *serverState, status *domain.ServerStatus) {
	// Enrich UDP players and recount humans/bots
	status.HumanCount = 0
	status.BotCount = 0

	for i := range status.Players {
		player := &status.Players[i]
		var client *clientState

		// Match by ClientNum if available (from enhanced UDP response)
		if player.ClientNum >= 0 {
			client = state.clients[player.ClientNum]
		}

		if client != nil {
			// Found matching tracked client. Bots have canonical "BOT:..."
			// GUIDs; the isBot flag is authoritative once ClientUserinfo
			// has fired. An empty guid means we haven't seen userinfo yet —
			// keep the "assume bot" fallback from below.
			player.IsBot = client.isBot || client.guid == ""
			player.Skill = client.skill
			if !client.joinedAt.IsZero() {
				player.JoinedAt = client.joinedAt
			}
			player.Impressives = client.impressives
			player.Excellents = client.excellents
			player.Humiliations = client.humiliations
			player.Defends = client.defends
			player.Captures = client.captures
			player.Assists = client.assists
			player.Model = client.model
			player.IsVR = client.isVR
			player.PlayerID = client.getPlayerIDPtr()
			player.IsVerified = client.isVerified
			player.IsAdmin = client.isAdmin
		} else {
			// No tracked client yet - assume bot until GUID data arrives from logs
			player.IsBot = true
		}

		if player.IsBot {
			status.BotCount++
		} else {
			status.HumanCount++
		}
	}
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
		m.handleMatchChange(ctx, state, data.MapName, data.GameType, data.UUID, data.Settings["g_movement"], data.Settings["g_gameplay"], event.Timestamp, replayMode)

	case EventTypeWarmupEnd:
		// Persist match to DB now that real gameplay is starting
		if state.match != nil {
			state.match.StartedAt = event.Timestamp
			if !replayMode && state.match.UUID != "" {
				m.writer.Publish(domain.FactEvent{
					Type:      domain.FactMatchStart,
					ServerID:  state.server.ID,
					Timestamp: event.Timestamp,
					Data: domain.MatchStartData{
						MatchUUID: state.match.UUID,
						MapName:   state.match.MapName,
						GameType:  state.match.GameType,
						Movement:  state.match.Movement,
						Gameplay:  state.match.Gameplay,
						StartedAt: event.Timestamp,
					},
				})
				state.matchStarted = true
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
		if data.State == "intermission" && state.pendingExit != nil &&
			state.match != nil && state.match.UUID != "" &&
			!state.matchFlushed && state.match.EndedAt == nil {
			players := m.buildMatchEndPlayers(state, true)
			m.writer.Publish(domain.FactEvent{
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

		// Resolve identity through the hub writer. Replay mode is a
		// read-only lookup; live mode upserts.
		if client.guid != "" {
			var id hub.PlayerIdentity
			var err error
			switch {
			case replayMode:
				id, err = m.writer.LookupPlayerIdentity(ctx, client.guid)
			case data.IsBot:
				id, err = m.writer.UpsertBotPlayerIdentity(ctx, data.Name, client.cleanName, event.Timestamp)
			default:
				id, err = m.writer.UpsertPlayerIdentity(ctx, data.GUID, data.Name, client.cleanName, event.Timestamp, data.IsVR)
			}
			if err != nil {
				log.Printf("Error resolving player identity for GUID %s: %v", client.guid, err)
			} else if id.Found {
				client.playerID = id.PlayerID
				client.isVerified = id.IsVerified
				client.isAdmin = id.IsAdmin
			}
		}

	case EventTypeClientBegin:
		data := event.Data.(ClientConnectData)
		if client, ok := state.clients[data.ClientID]; ok {
			client.began = true

			// Track whether this is a new connection (for greeting logic)
			isNewSession := false

			// Create session for human players only (require resolved identity, not a bot).
			// Sessions track player presence on server; bots don't need tracking.
			if client.playerID > 0 && !client.isBot && client.guid != "" {
				// Existing open session? (map change continuation)
				openSession, _ := m.writer.LookupOpenSession(ctx, serverID, client.guid)
				adopted := openSession != nil && !openSession.JoinedAt.After(event.Timestamp)

				if !adopted {
					// Replay idempotency: exact-timestamp match
					existing, _ := m.writer.LookupSessionByJoinTime(ctx, serverID, client.guid, event.Timestamp)
					if existing != nil {
						adopted = true
					} else {
						// Closed session that was active at this time (replay edge case)
						activeSession, _ := m.writer.LookupSessionActiveAt(ctx, serverID, client.guid, event.Timestamp)
						if activeSession != nil {
							adopted = true
						}
					}
				}

				if !adopted {
					// Publish player_join; hub writer creates the session row.
					m.writer.Publish(domain.FactEvent{
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
							JoinedAt:  event.Timestamp,
						},
					})
					isNewSession = true
				}
				// Note: match_player_stats row is created at flush time (disconnect or match end)
				// to avoid issues with matchID=0 before match is persisted to DB
			}

			// Emit player join event (skip in replay mode)
			if !replayMode {
				m.emitEvent(domain.Event{
					Type:      domain.EventPlayerJoin,
					ServerID:  serverID,
					Timestamp: event.Timestamp,
					Data: domain.PlayerJoinEvent{
						Player: domain.PlayerStatus{
							GUID:       client.guid,
							Name:       client.name,
							CleanName:  client.cleanName,
							IsBot:      client.isBot,
							IsVR:       client.isVR,
							IsVerified: client.isVerified,
							IsAdmin:    client.isAdmin,
							Team:       client.team,
							JoinedAt:   event.Timestamp,
						},
					},
				})

				// Greet human players on initial connection only (skip map changes, bots, startup)
				if m.startupComplete && isNewSession && client.guid != "" {
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

	case EventTypeClientDisconnect:
		data := event.Data.(ClientDisconnectData)
		if client, ok := state.clients[data.ClientID]; ok {
			// Publish player_leave for human players. The hub writer resolves
			// the matching open session by (server, guid) and closes it;
			// idempotent against duplicate/replay events.
			if !client.isBot && client.guid != "" {
				duration := int(event.Timestamp.Sub(client.joinedAt).Seconds())
				if duration < 0 {
					duration = 0
				}
				m.writer.Publish(domain.FactEvent{
					Type:      domain.FactPlayerLeave,
					ServerID:  serverID,
					Timestamp: event.Timestamp,
					Data: domain.PlayerLeaveData{
						GUID:            client.guid,
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

			// Emit player leave event (skip in replay mode)
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

			// Clean up Trinity handshake state
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

		// Emit frag event (skip in replay mode)
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

			// Use team scores from log (authoritative source)
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
		// Skip fake ShutdownGame that occurs at same timestamp as InitGame (part of warmup sequence)
		if event.Timestamp.Equal(state.lastInitGame) {
			break
		}

		// End current match if any
		if state.match != nil {
			if state.matchFlushed {
				// Already flushed at intermission — just end match bookkeeping
			} else if replayMode {
				// Replay mode: only flush/end if there was a proper Exit event (pendingExit set).
				// If pendingExit is nil, this might be a mid-match server restart, and the match
				// could still be ongoing. EndAllOpenMatchesExcept cleans up truly orphaned matches.
				if state.pendingExit != nil && state.match.UUID != "" {
					existing, err := m.writer.LookupMatch(ctx, state.match.UUID)
					if err != nil {
						log.Printf("Error looking up match by UUID during replay: %v", err)
					} else if existing != nil && existing.EndedAt == nil {
						m.writer.Publish(domain.FactEvent{
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
					}
				}
			} else if state.matchStarted && state.match.UUID != "" && state.match.EndedAt == nil {
				// Live mode: flush all player stats and end match
				if state.pendingExit != nil {
					// Normal match end: Exit event was received, scores have been captured
					m.writer.Publish(domain.FactEvent{
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
					m.writer.Publish(domain.FactEvent{
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
		// Clear all client state (including kills/deaths/score counters)
		state.clients = make(map[int]*clientState)
		state.previousClients = make(map[string]*clientState)

	case EventTypeFlagCapture:
		data := event.Data.(FlagCaptureData)
		// Track capture in memory for real-time display
		if client, ok := state.clients[data.ClientID]; ok {
			client.captures++
		}
		// Emit event (skip in replay mode) - DB write happens at flush time
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
		// Emit event (skip in replay mode) - DB write happens at flush time
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
						clientID:   client.clientID,
						playerID:   client.playerID,
						name:       client.name,
						cleanName:  client.cleanName,
						guid:       client.guid,
						model:      client.model,
						isBot:      client.isBot,
						isVR:       client.isVR,
						isVerified: client.isVerified,
						isAdmin:    client.isAdmin,
						skill:      client.skill,
						team:       data.NewTeam,
						joinedAt:   event.Timestamp,
						ipAddress:  client.ipAddress,
						began:      client.began,
						score:      &initialScore,
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

		// Emit event (skip in replay mode)
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
		// Track assist in memory for real-time display - DB write happens at flush time
		if client, ok := state.clients[data.ClientID]; ok {
			client.assists++
		}

	case EventTypeAward:
		// Only track awards during active gameplay (not warmup/waiting/intermission)
		if state.matchState != "active" && state.matchState != "overtime" {
			break
		}
		data := event.Data.(AwardData)
		if client, ok := state.clients[data.ClientID]; ok {
			// Track in memory for real-time display - DB write happens at flush time
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

			// Emit award event (skip in replay mode)
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
		// Server startup without preceding shutdown indicates crash recovery.
		// Hub writer closes any open sessions that started before this timestamp.
		m.writer.Publish(domain.FactEvent{
			Type:      domain.FactServerStartup,
			ServerID:  serverID,
			Timestamp: event.Timestamp,
			Data:      domain.ServerStartupData{StartedAt: event.Timestamp},
		})

	case EventTypeServerShutdown:
		// Clean server shutdown - hub writer closes open sessions that
		// started before this timestamp (filter avoids closing sessions
		// from later events during replay).
		m.writer.Publish(domain.FactEvent{
			Type:      domain.FactServerShutdown,
			ServerID:  serverID,
			Timestamp: event.Timestamp,
			Data:      domain.ServerShutdownData{ShutdownAt: event.Timestamp},
		})

	case EventTypeCvarChange:
		data := event.Data.(CvarChangeData)
		if state.match != nil && state.match.UUID != "" {
			switch data.Key {
			case "g_movement":
				state.match.Movement = data.Value
				m.writer.Publish(domain.FactEvent{
					Type:      domain.FactMatchSettingsUpdate,
					ServerID:  state.server.ID,
					Timestamp: event.Timestamp,
					Data:      domain.MatchSettingsUpdateData{MatchUUID: state.match.UUID, Movement: data.Value},
				})
			case "g_gameplay":
				state.match.Gameplay = data.Value
				m.writer.Publish(domain.FactEvent{
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
func (m *ServerManager) handleMatchChange(ctx context.Context, state *serverState, mapName string, gameType int, uuid string, movement string, gameplay string, ts time.Time, replayMode bool) {
	// Skip duplicate InitGame at same timestamp (Q3 sometimes logs it twice on server restart)
	if ts.Equal(state.lastInitGame) {
		return
	}
	state.lastInitGame = ts

	gameTypeStr := domain.GameTypeFromInt(gameType)

	// In replay mode, look up existing match from DB (never publish new MatchStart)
	if replayMode {
		prevMatch := state.match
		state.match = nil
		state.matchStarted = false

		if existing, err := m.writer.LookupMatch(ctx, uuid); err == nil && existing != nil {
			if existing.Movement == "" && movement != "" {
				existing.Movement = movement
				m.writer.Publish(domain.FactEvent{
					Type: domain.FactMatchSettingsUpdate, ServerID: state.server.ID, Timestamp: ts,
					Data: domain.MatchSettingsUpdateData{MatchUUID: uuid, Movement: movement},
				})
			}
			if existing.Gameplay == "" && gameplay != "" {
				existing.Gameplay = gameplay
				m.writer.Publish(domain.FactEvent{
					Type: domain.FactMatchSettingsUpdate, ServerID: state.server.ID, Timestamp: ts,
					Data: domain.MatchSettingsUpdateData{MatchUUID: uuid, Gameplay: gameplay},
				})
			}
			state.match = existing
			// A row in DB means WarmupEnd fired in a prior run (the writer
			// only creates matches at MatchStart). Mark started so the
			// live-mode Shutdown that follows doesn't log "Discarding".
			state.matchStarted = true
		}

		// If UUID lookup failed and we had a previous match with different UUID,
		// the previous match was interrupted (server crash). End it.
		if state.match == nil && prevMatch != nil && prevMatch.UUID != "" &&
			prevMatch.UUID != uuid && prevMatch.EndedAt == nil {
			m.writer.Publish(domain.FactEvent{
				Type: domain.FactMatchCrashed, ServerID: state.server.ID, Timestamp: ts,
				Data: domain.MatchCrashedData{MatchUUID: prevMatch.UUID, EndedAt: ts},
			})
		}

		// If no match found in DB, leave state.match = nil
		// Stats will be reconstructed at ShutdownGame when match is created
		state.clients = make(map[int]*clientState)
		state.previousClients = make(map[string]*clientState)
		state.matchFlushed = false
		state.matchState = ""
		state.warmupDuration = 0
		return
	}

	// Fallback close: if previous match was never flushed (no ShutdownGame seen),
	// end it now as crashed with whatever player stats we have.
	if !state.matchFlushed && state.matchStarted && state.match != nil &&
		state.match.UUID != "" && state.match.EndedAt == nil {
		m.writer.Publish(domain.FactEvent{
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

	// Check if a match with this UUID already exists in the database
	// This handles the "late replay" case where an ongoing match's InitGame
	// is processed as live mode after a collector restart.
	existing, err := m.writer.LookupMatch(ctx, uuid)
	if err != nil {
		log.Printf("Error checking for existing match UUID: %v", err)
	}

	if existing != nil {
		if existing.Movement == "" && movement != "" {
			existing.Movement = movement
			m.writer.Publish(domain.FactEvent{
				Type: domain.FactMatchSettingsUpdate, ServerID: state.server.ID, Timestamp: ts,
				Data: domain.MatchSettingsUpdateData{MatchUUID: uuid, Movement: movement},
			})
		}
		if existing.Gameplay == "" && gameplay != "" {
			existing.Gameplay = gameplay
			m.writer.Publish(domain.FactEvent{
				Type: domain.FactMatchSettingsUpdate, ServerID: state.server.ID, Timestamp: ts,
				Data: domain.MatchSettingsUpdateData{MatchUUID: uuid, Gameplay: gameplay},
			})
		}
		if existing.EndedAt == nil {
			// Match is still open - adopt it and sweep any other open matches
			log.Printf("Resuming existing open match UUID=%s on %s", uuid, mapName)
			state.match = existing
			state.matchStarted = true // already past warmup in DB
			if err := m.writer.EndAllOpenMatchesExcept(ctx, state.server.ID, ts, existing.ID); err != nil {
				log.Printf("Error ending other open matches: %v", err)
			}
		} else {
			// Match already ended - duplicate InitGame, load existing
			log.Printf("Match UUID=%s already ended, loading existing", uuid)
			state.match = existing
			state.matchStarted = false // already finished; no more stats expected
		}

		// Clear client state for state rebuild
		state.clients = make(map[int]*clientState)
		state.previousClients = make(map[string]*clientState)
		state.matchFlushed = false
		state.matchState = ""
		state.warmupDuration = 0
		return
	}

	// UUID is new - genuinely a new match. Sweep any orphans on this server.
	if err := m.writer.EndAllOpenMatchesExcept(ctx, state.server.ID, ts, 0); err != nil {
		log.Printf("Error ending open matches: %v", err)
	}

	// Create in-memory match object (defer DB persistence until WarmupEnd)
	// This avoids creating orphaned match records for instant restarts or
	// matches that never leave warmup/waiting state
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

	// Clear client state and reset match state for new map
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


// savePreviousClient accumulates stats from a completed stint into previousClients.
// One entry per GUID — counters are added, metadata is updated to latest.
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
		// Update to latest metadata
		prev.clientID = client.clientID
		prev.team = client.team
		prev.model = client.model
	} else {
		state.previousClients[client.guid] = client
	}
}

// buildMatchEndPlayers assembles the per-player stats for a MatchEndData
// event. Ordering mirrors the legacy flush: previous stints first (so
// connected clients' metadata wins when duplicates land on the same row),
// connected players last. Bot/unverified skip rules match the legacy.
func (m *ServerManager) buildMatchEndPlayers(state *serverState, computeVictory bool) []domain.MatchEndPlayer {
	var maxFFAScore int
	var hasFFAScores bool
	if computeVictory {
		maxFFAScore, hasFFAScores = computeMaxScore(state.clients)
	}

	// On servers with Trinity handshake enabled, unverified players get auto-kicked,
	// so skip saving their stats entirely.
	skipUnverified := m.handshakeRequired(state)

	var players []domain.MatchEndPlayer

	// Previous stints first (completed=false)
	for _, client := range state.previousClients {
		if skipUnverified && !client.isBot && !client.isVerified {
			continue
		}
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
		if skipUnverified && !client.isBot && !client.isVerified {
			continue
		}
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

// emitEvent sends an event to the event channel
func (m *ServerManager) emitEvent(event domain.Event) {
	select {
	case m.events <- event:
	default:
		// Channel full, drop event
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
		client.playerID = reply.NewPlayerID
		m.sendPrint(serverID, clientID, "^2Link successful! ^7This identity has been linked to your account.")
		log.Printf("Link successful: GUID %s merged into player %d via code %s", client.guid, reply.NewPlayerID, code)
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

	// Validate client has a resolvable identity
	if client.guid == "" || client.playerID == 0 {
		m.sendPrint(serverID, clientID, "^1Error: Current identity unknown. Try reconnecting.")
		return
	}

	reply, err := m.rpc.Claim(ctx, hub.ClaimRequest{GUID: client.guid, PlayerID: client.playerID})
	if err != nil {
		log.Printf("claim RPC error for player %d: %v", client.playerID, err)
		m.sendPrint(serverID, clientID, "^1Error generating claim code. Please try again.")
		return
	}

	switch reply.Status {
	case hub.ClaimOK:
		m.sendPrint(serverID, clientID, fmt.Sprintf("Your claim code is: ^3%s^7 - Visit ^5trinity.run ^7to claim this identity. Expires in 30 minutes.", reply.Code))
		log.Printf("Claim code %s generated for player %d on server %d", reply.Code, client.playerID, serverID)
	case hub.ClaimAlreadyClaimed:
		m.sendPrint(serverID, clientID, "^3This identity is already linked to an account.")
	case hub.ClaimUnknownPlayer:
		m.sendPrint(serverID, clientID, "^1Error: Could not identify your player record. Try reconnecting.")
	default:
		log.Printf("claim RPC error: %s", reply.Message)
		m.sendPrint(serverID, clientID, "^1Error generating claim code. Please try again.")
	}
}

// sendPrint sends a console print to a player via RCON (runs async).
func (m *ServerManager) sendPrint(serverID int64, clientID int, message string) {
	go m.sendPrintSync(serverID, clientID, message)
}

// sendPrintSync sends a console print to a player via RCON synchronously.
// Use this when ordering matters (e.g., sending multiple messages in sequence).
func (m *ServerManager) sendPrintSync(serverID int64, clientID int, message string) {
	cmd := fmt.Sprintf("sv_cmd print %d ^7%s\\n", clientID, message)
	log.Printf("Sending print to client %d: %q", clientID, message)
	if _, err := m.ExecuteRcon(serverID, cmd); err != nil {
		log.Printf("Error sending print to client %d on server %d: %v", clientID, serverID, err)
	}
}

// sendCenterPrint sends a center print to a player via RCON.
func (m *ServerManager) sendCenterPrint(serverID int64, clientID int, message string) {
	// Replace real newlines with literal \n so they survive RCON transport
	// and get interpreted as line breaks by the engine's centerprint renderer.
	escaped := strings.ReplaceAll(message, "\n", "\\n")
	cmd := fmt.Sprintf("sv_cmd cp %d ^7%s", clientID, escaped)
	if _, err := m.ExecuteRcon(serverID, cmd); err != nil {
		log.Printf("Error sending cp to client %d on server %d: %v", clientID, serverID, err)
	}
}

// greetPlayer sends a welcome message to a player when they join
// performGreet issues the greet RPC to the hub writer and applies the
// reply: either an auth-fail rcon kick or a formatted welcome message.
// Auth is nil for non-handshake servers and handshake-required servers
// whose players skipped the handshake. On auth_result=failed, the
// collector sends trinity_auth_fail and skips the welcome.
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
		return
	}

	// Update cached identity on the client record. Best-effort: if the
	// client has already disconnected by the time the reply lands, this
	// is a no-op — the state lookup will miss but we still proceed to
	// send rcon, which is idempotent.
	m.mu.Lock()
	for _, state := range m.servers {
		if state.server.ID != serverID {
			continue
		}
		if client, ok := state.clients[clientID]; ok {
			if reply.PlayerID != 0 {
				client.playerID = reply.PlayerID
			}
			client.isVerified = reply.IsVerified
			client.isAdmin = reply.IsAdmin
		}
	}
	m.mu.Unlock()

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
}

// isNumeric checks if a string contains only digits
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// trimSpace removes leading and trailing whitespace
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

// indexSpace returns the index of the first space character, or -1 if not found
func indexSpace(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			return i
		}
	}
	return -1
}

