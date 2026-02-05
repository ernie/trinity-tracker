package collector

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/ernie/trinity-tools/internal/config"
	"github.com/ernie/trinity-tools/internal/domain"
	"github.com/ernie/trinity-tools/internal/storage"
)

// ServerManager orchestrates polling and log parsing for all servers
type ServerManager struct {
	cfg      *config.Config
	store    *storage.Store
	q3client *Q3Client
	events   chan domain.Event

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
	clients          map[int]*clientState // client ID -> client state
	lastInitGame     time.Time            // dedupe InitGame and skip fake ShutdownGame at same timestamp
	matchState       string               // "waiting", "warmup", "active", "intermission"
	warmupDuration   int                  // warmup duration in seconds (set when warmup starts)
	pendingExit      *string              // exit reason from Exit event (deferred until scores captured)
	pendingExitAt    time.Time            // timestamp of Exit event
	pendingRedScore  *int                 // team scores captured at Exit time (before server resets)
	pendingBlueScore *int
}

// gauntletVictim tracks victim info for humiliation awards
type gauntletVictim struct {
	name     string
	playerID int64
}

// clientState tracks a connected client
type clientState struct {
	clientID           int
	playerGUID         int64 // ID of the player_guids record
	playerID           int64 // ID of the players record (for events)
	sessionID          int64
	name               string
	cleanName          string
	guid               string
	model              string // player model (e.g., "sarge/krusade")
	isBot              bool
	isVR               bool
	skill              float64 // bot skill level (1-5), 0 if human
	team               int
	joinedAt           time.Time
	ipAddress          string          // client IP address from ClientConnect
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

// NewServerManager creates a new manager
func NewServerManager(cfg *config.Config, store *storage.Store) *ServerManager {
	return &ServerManager{
		cfg:      cfg,
		store:    store,
		q3client: NewQ3Client(),
		events:   make(chan domain.Event, 100),
		servers:  make(map[int64]*serverState),
		tailers:  make(map[int64]*LogTailer),
		done:     make(chan struct{}),
	}
}

// Events returns the event channel for WebSocket broadcasting
func (m *ServerManager) Events() <-chan domain.Event {
	return m.events
}

// Start initializes all servers and begins polling
func (m *ServerManager) Start(ctx context.Context) error {
	// Register servers from config and replay logs synchronously
	for _, srv := range m.cfg.Q3Servers {
		dbSrv := &domain.Server{
			Name:    srv.Name,
			Address: srv.Address,
			LogPath: srv.LogPath,
		}
		if err := m.store.UpsertServer(ctx, dbSrv); err != nil {
			return err
		}

		fullSrv, err := m.store.GetServerByID(ctx, dbSrv.ID)
		if err != nil {
			return err
		}

		m.servers[dbSrv.ID] = &serverState{
			server:  *fullSrv,
			clients: make(map[int]*clientState),
		}

		// Replay log events synchronously (one server at a time to avoid DB lock contention)
		if srv.LogPath != "" {
			startAfter := time.Time{} // epoch - replay all
			if fullSrv.LastMatchEndedAt != nil {
				startAfter = *fullSrv.LastMatchEndedAt
			}

			tailer := NewLogTailer(srv.LogPath, nil)
			if _, err := tailer.OpenFile(); err != nil {
				log.Printf("Warning: failed to open log file for %s: %v", srv.Name, err)
				continue
			}

			log.Printf("Replaying log for %s from %v", srv.Name, startAfter)
			serverID := dbSrv.ID
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
				m.tailers[dbSrv.ID] = tailer
				m.wg.Add(1)
				go m.processLogEvents(ctx, dbSrv.ID, tailer)
			}
		}
	}

	// Start UDP polling
	m.wg.Add(1)
	go m.pollLoop(ctx)

	// Start expired link code cleanup
	m.wg.Add(1)
	go m.linkCodeCleanupLoop(ctx)

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
			// Found matching tracked client - human if they have GUID, bot otherwise
			player.IsBot = client.guid == ""
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
		m.handleMapChange(ctx, state, data.MapName, data.GameType, data.UUID, event.Timestamp, replayMode)

	case EventTypeWarmupEnd:
		// Persist match to DB now that real gameplay is starting
		if state.match != nil {
			state.match.StartedAt = event.Timestamp
			if !replayMode {
				if state.match.ID == 0 {
					// Match not yet persisted - create it now
					if err := m.store.CreateMatch(ctx, state.match); err != nil {
						log.Printf("Error creating match: %v", err)
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

	case EventTypeClientConnect:
		data := event.Data.(ClientConnectData)
		state.clients[data.ClientID] = &clientState{
			clientID:  data.ClientID,
			joinedAt:  event.Timestamp,
			ipAddress: data.IPAddress,
		}

	case EventTypeClientUserinfo:
		data := event.Data.(ClientUserinfoData)
		client, ok := state.clients[data.ClientID]
		if !ok {
			client = &clientState{
				clientID: data.ClientID,
				joinedAt: event.Timestamp,
			}
			state.clients[data.ClientID] = client
		}
		client.name = data.Name
		client.cleanName = domain.CleanQ3Name(data.Name)
		client.guid = data.GUID
		client.isBot = data.IsBot
		client.isVR = data.IsVR
		client.skill = data.Skill
		client.team = data.Team
		client.model = data.Model

		// Look up or create player GUID in database
		if replayMode {
			// During replay, just look up existing playerGUID (don't create)
			var guidToLookup string
			if data.IsBot {
				guidToLookup = "BOT:" + client.cleanName
			} else {
				guidToLookup = data.GUID
			}
			if guidToLookup != "" {
				pg, err := m.store.GetPlayerGUIDByGUID(ctx, guidToLookup)
				if err == nil && pg != nil {
					client.playerGUID = pg.ID
					client.playerID = pg.PlayerID
				}
			}
		} else {
			// Live mode: upsert playerGUID
			if data.IsBot {
				// Create/update bot player with synthetic GUID
				pg, err := m.store.UpsertBotPlayerGUID(ctx, data.Name, client.cleanName, event.Timestamp)
				if err != nil {
					log.Printf("Error upserting bot player GUID: %v", err)
				} else {
					client.playerGUID = pg.ID
					client.playerID = pg.PlayerID
				}
			} else if data.GUID != "" {
				// Human player with real GUID
				pg, err := m.store.UpsertPlayerGUID(ctx, data.GUID, data.Name, client.cleanName, event.Timestamp, data.IsVR)
				if err != nil {
					log.Printf("Error upserting player GUID: %v", err)
				} else {
					client.playerGUID = pg.ID
					client.playerID = pg.PlayerID
				}
			}
		}

	case EventTypeClientBegin:
		data := event.Data.(ClientConnectData)
		if client, ok := state.clients[data.ClientID]; ok {
			// Track whether this is a new connection (for greeting logic)
			isNewSession := false

			// Create session for human players only (require valid player GUID, not a bot)
			// Sessions track player presence on server; bots don't need tracking
			if client.playerGUID > 0 && !client.isBot {
				// First check for existing OPEN session that started BEFORE OR AT this event
				// This handles map change continuation: the same player reconnects after
				// ShutdownGame clears state.clients, but their session is still open in DB
				openSession, _ := m.store.GetOpenSessionForPlayer(ctx, client.playerGUID, serverID)

				if openSession != nil && !openSession.JoinedAt.After(event.Timestamp) {
					// Continue existing session (map change case, or exact timestamp match)
					client.sessionID = openSession.ID
				} else {
					// No usable open session - check for exact timestamp match (replay idempotency)
					existing, _ := m.store.GetSessionByPlayerAndJoinTime(ctx, client.playerGUID, serverID, event.Timestamp)
					if existing != nil {
						client.sessionID = existing.ID
					} else {
						// Check if there's a closed session that was active at this time
						// This handles map change ClientBegins during replay where the session
						// was already closed by a later ClientDisconnect
						activeSession, _ := m.store.GetSessionActiveAt(ctx, client.playerGUID, serverID, event.Timestamp)
						if activeSession != nil {
							client.sessionID = activeSession.ID
						} else {
							// Create new session - covers both live mode and replay of events
							// that occurred while collector was down. Idempotency is ensured by
							// the timestamp check above (won't create duplicates).
							session := &domain.Session{
								PlayerGUIDID: client.playerGUID,
								ServerID:     serverID,
								JoinedAt:     event.Timestamp,
								IPAddress:    client.ipAddress,
							}
							if err := m.store.CreateSession(ctx, session); err != nil {
								log.Printf("Error creating session: %v", err)
							} else {
								client.sessionID = session.ID
								isNewSession = true
							}
						}
					}
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
							Name:      client.name,
							CleanName: client.cleanName,
							IsBot:     client.isBot,
							IsVR:      client.isVR,
							Team:      client.team,
							JoinedAt:  event.Timestamp,
						},
						PlayerID: client.getPlayerIDPtr(),
					},
				})

				// Greet human players on initial connection only (skip map changes, bots, startup)
				if m.startupComplete && isNewSession && client.playerID != 0 {
					go m.greetPlayer(ctx, serverID, data.ClientID, client.playerID, client.name)
				}
			}
		}

	case EventTypeClientDisconnect:
		data := event.Data.(ClientDisconnectData)
		if client, ok := state.clients[data.ClientID]; ok {
			// End session - works in both live and replay mode for "collector was down" scenario
			// EndSession is idempotent (WHERE left_at IS NULL), safe to call multiple times
			if client.sessionID > 0 {
				if err := m.store.EndSession(ctx, client.sessionID, event.Timestamp); err != nil {
					log.Printf("Error ending session: %v", err)
				}
			}

			// Flush match player stats
			// If pendingExit is set, the match is over (intermission) so player completed it
			// Players who leave mid-match don't get victory credit
			// Skip pure spectators (team 3 with no kills/deaths)
			if client.playerGUID > 0 && (client.team != 3 || client.frags > 0 || client.deaths > 0) {
				if matchID := m.getMatchID(ctx, state); matchID > 0 {
					completed := state.pendingExit != nil
					var team *int
					if client.team > 0 {
						team = &client.team
					}
					// Determine if player joined late (after warmup ended)
					joinedLate := state.matchState == "active" && client.joinedAt.After(state.match.StartedAt)
					if err := m.store.FlushMatchPlayerStats(ctx, matchID, client.playerGUID, data.ClientID,
						client.frags, client.deaths, completed, client.score, team, client.model, client.skill, false,
						client.captures, client.flagReturns, client.assists, client.impressives,
						client.excellents, client.humiliations, client.defends,
						client.isBot, joinedLate, client.joinedAt, client.isVR); err != nil {
						log.Printf("Error flushing match player stats: %v", err)
					}
				}
			}

			// Emit player leave event (skip in replay mode)
			if !replayMode {
				m.emitEvent(domain.Event{
					Type:      domain.EventPlayerLeave,
					ServerID:  serverID,
					Timestamp: event.Timestamp,
					Data: domain.PlayerLeaveEvent{
						PlayerName: client.name,
						PlayerID:   client.getPlayerIDPtr(),
					},
				})
			}

			delete(state.clients, data.ClientID)
		}

	case EventTypeFrag:
		data := event.Data.(FragEventData)

		// Only track frags/deaths during active match (not warmup/waiting/intermission)
		// Note: We track stats even during replay so we can flush them if match wasn't completed
		if state.matchState == "active" {
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
					var victimPlayerID int64
					if victim, ok := state.clients[data.VictimID]; ok {
						victimPlayerID = victim.playerID
					}
					fragger.lastGauntletVictim = &gauntletVictim{
						name:     data.VictimName,
						playerID: victimPlayerID,
					}
				}
			}
		}

		// Emit frag event (skip in replay mode)
		if !replayMode {
			var fraggerPlayerID, victimPlayerID *int64
			if fragger, ok := state.clients[data.FraggerID]; ok && fragger.playerID > 0 {
				fraggerPlayerID = &fragger.playerID
			}
			if victim, ok := state.clients[data.VictimID]; ok && victim.playerID > 0 {
				victimPlayerID = &victim.playerID
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventFrag,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.FragEvent{
					Fragger:         data.FraggerName,
					Victim:          data.VictimName,
					Weapon:          data.Weapon,
					FraggerPlayerID: fraggerPlayerID,
					VictimPlayerID:  victimPlayerID,
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
			if replayMode {
				// Replay mode: only flush/end if there was a proper Exit event (pendingExit set)
				// If pendingExit is nil, this might be a mid-match server restart, and the match
				// could still be ongoing. EndAllOpenMatches will clean up truly orphaned matches.
				if state.pendingExit != nil && state.match.UUID != "" {
					existing, err := m.store.GetMatchByUUID(ctx, state.match.UUID)
					if err != nil {
						log.Printf("Error looking up match by UUID during replay: %v", err)
					} else if existing != nil && existing.EndedAt == nil {
						// Match exists and is open - flush stats and end it
						maxFFAScore := computeMaxScore(state.clients)
						for clientID, client := range state.clients {
							if client.playerGUID > 0 && (client.team != 3 || client.frags > 0 || client.deaths > 0) {
								var team *int
								if client.team > 0 {
									team = &client.team
								}
								victory := isMatchWinner(client, state, maxFFAScore)
								joinedLate := client.joinedAt.After(existing.StartedAt)
								m.store.FlushMatchPlayerStats(ctx, existing.ID, client.playerGUID, clientID,
									client.frags, client.deaths, true, client.score, team, client.model, client.skill, victory,
									client.captures, client.flagReturns, client.assists, client.impressives,
									client.excellents, client.humiliations, client.defends,
									client.isBot, joinedLate, client.joinedAt, client.isVR)
							}
						}
						m.store.EndMatch(ctx, existing.ID, state.pendingExitAt, *state.pendingExit, state.pendingRedScore, state.pendingBlueScore)
					}
				}
			} else {
				// Live mode: flush individual player stats and end match
				matchID := m.getMatchID(ctx, state)

				if matchID > 0 {
					if state.pendingExit != nil {
						// Normal match end: Exit event was received, scores have been captured
						maxFFAScore := computeMaxScore(state.clients)
						for clientID, client := range state.clients {
							if client.playerGUID > 0 {
								var team *int
								if client.team > 0 {
									team = &client.team
								}
								victory := isMatchWinner(client, state, maxFFAScore)
								joinedLate := state.match != nil && client.joinedAt.After(state.match.StartedAt)
								if err := m.store.FlushMatchPlayerStats(ctx, matchID, client.playerGUID, clientID,
									client.frags, client.deaths, true, client.score, team, client.model, client.skill, victory,
									client.captures, client.flagReturns, client.assists, client.impressives,
									client.excellents, client.humiliations, client.defends,
									client.isBot, joinedLate, client.joinedAt, client.isVR); err != nil {
									log.Printf("Error flushing match player stats: %v", err)
								}
							}
						}

						if err := m.store.EndMatch(ctx, matchID, state.pendingExitAt, *state.pendingExit, state.pendingRedScore, state.pendingBlueScore); err != nil {
							log.Printf("Error ending match: %v", err)
						}
					} else {
						// Abnormal shutdown: no Exit event, so no scores or victories
						for clientID, client := range state.clients {
							if client.playerGUID > 0 {
								var team *int
								if client.team > 0 {
									team = &client.team
								}
								joinedLate := state.match != nil && client.joinedAt.After(state.match.StartedAt)
								m.store.FlushMatchPlayerStats(ctx, matchID, client.playerGUID, clientID,
									client.frags, client.deaths, false, nil, team, client.model, client.skill, false,
									client.captures, client.flagReturns, client.assists, client.impressives,
									client.excellents, client.humiliations, client.defends,
									client.isBot, joinedLate, client.joinedAt, client.isVR)
							}
						}
						m.store.EndMatch(ctx, matchID, event.Timestamp, "shutdown", nil, nil)
					}
				} else if state.match != nil && state.match.ID == 0 {
					// Match never reached active play (no WarmupEnd) - discard without persisting
					log.Printf("Discarding match %s: never reached active play", state.match.UUID)
				}
			}
		}
		state.match = nil
		state.pendingExit = nil
		state.pendingRedScore = nil
		state.pendingBlueScore = nil
		// Clear all client state (including kills/deaths/score counters)
		state.clients = make(map[int]*clientState)

	case EventTypeFlagCapture:
		data := event.Data.(FlagCaptureData)
		// Track capture in memory for real-time display
		if client, ok := state.clients[data.ClientID]; ok {
			client.captures++
		}
		// Emit event (skip in replay mode) - DB write happens at flush time
		if !replayMode {
			var playerID *int64
			if client, ok := state.clients[data.ClientID]; ok && client.playerID > 0 {
				playerID = &client.playerID
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventFlagCapture,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.FlagCaptureEvent{
					ClientNum:  data.ClientID,
					PlayerName: data.Name,
					Team:       data.Team,
					PlayerID:   playerID,
				},
			})
		}

	case EventTypeFlagTaken:
		data := event.Data.(FlagTakenData)
		// Skip events in replay mode
		if !replayMode {
			var playerID *int64
			if client, ok := state.clients[data.ClientID]; ok && client.playerID > 0 {
				playerID = &client.playerID
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventFlagTaken,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.FlagTakenEvent{
					ClientNum:  data.ClientID,
					PlayerName: data.Name,
					Team:       data.Team,
					PlayerID:   playerID,
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
			var playerID *int64
			// Auto-returns have ClientID == -1 and no player associated
			if data.ClientID >= 0 {
				if client, ok := state.clients[data.ClientID]; ok && client.playerID > 0 {
					playerID = &client.playerID
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
					PlayerID:   playerID,
				},
			})
		}

	case EventTypeFlagDrop:
		data := event.Data.(FlagDropData)
		// Skip events in replay mode
		if !replayMode {
			var playerID *int64
			if client, ok := state.clients[data.ClientID]; ok && client.playerID > 0 {
				playerID = &client.playerID
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventFlagDrop,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.FlagDropEvent{
					ClientNum:  data.ClientID,
					PlayerName: data.Name,
					Team:       data.Team,
					PlayerID:   playerID,
				},
			})
		}

	case EventTypeObeliskDestroy:
		data := event.Data.(ObeliskDestroyData)
		// Skip events in replay mode
		if !replayMode {
			var playerID *int64
			if client, ok := state.clients[data.AttackerID]; ok && client.playerID > 0 {
				playerID = &client.playerID
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventObeliskDestroy,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.ObeliskDestroyEvent{
					AttackerName: data.Attacker,
					Team:         data.Team,
					PlayerID:     playerID,
				},
			})
		}

	case EventTypeSkullScore:
		data := event.Data.(SkullScoreData)
		// Skip events in replay mode
		if !replayMode {
			var playerID *int64
			if client, ok := state.clients[data.ClientID]; ok && client.playerID > 0 {
				playerID = &client.playerID
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventSkullScore,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.SkullScoreEvent{
					PlayerName: data.Name,
					Team:       data.Team,
					Skulls:     data.Skulls,
					PlayerID:   playerID,
				},
			})
		}

	case EventTypeTeamChange:
		data := event.Data.(TeamChangeData)
		if client, ok := state.clients[data.ClientID]; ok {
			oldTeam := client.team
			client.team = data.NewTeam

			// Flush stats when leaving a playing team (to spectator OR to different team)
			// The game resets score on any team switch, so we need to flush accumulated stats
			// Skip if coming from spectator (no stats to flush) or if team didn't actually change
			if oldTeam != 3 && oldTeam != data.NewTeam && client.playerGUID > 0 {
				if matchID := m.getMatchID(ctx, state); matchID > 0 {
					var team *int
					if oldTeam > 0 {
						team = &oldTeam
					}
					joinedLate := state.matchState == "active" && state.match != nil && client.joinedAt.After(state.match.StartedAt)
					// Flush with completed=false (switched teams mid-match), no victory
					m.store.FlushMatchPlayerStats(ctx, matchID, client.playerGUID, data.ClientID,
						client.frags, client.deaths, false, client.score, team, client.model, client.skill, false,
						client.captures, client.flagReturns, client.assists, client.impressives,
						client.excellents, client.humiliations, client.defends,
						client.isBot, joinedLate, client.joinedAt, client.isVR)
					// Reset in-memory counters after flushing
					client.frags = 0
					client.deaths = 0
					client.captures = 0
					client.flagReturns = 0
					client.assists = 0
					client.impressives = 0
					client.excellents = 0
					client.humiliations = 0
					client.defends = 0
					client.score = nil
				}
			}

			// Update joinedAt when starting a new playing stint
			// - Switching FROM spectator to a playing team
			// - Switching between playing teams (Red→Blue) after flush
			if data.NewTeam != 3 && (oldTeam == 3 || oldTeam != data.NewTeam) {
				client.joinedAt = event.Timestamp
			}
		}

		// Emit event (skip in replay mode)
		if !replayMode {
			var playerID *int64
			if client, ok := state.clients[data.ClientID]; ok && client.playerID > 0 {
				playerID = &client.playerID
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventTeamChange,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.TeamChangeEvent{
					PlayerName: data.Name,
					OldTeam:    data.OldTeam,
					NewTeam:    data.NewTeam,
					PlayerID:   playerID,
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
		// Only track awards during active match (not warmup/waiting/intermission)
		if state.matchState != "active" {
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
					PlayerID:   client.getPlayerIDPtr(),
				}

				// Include victim info for humiliation awards
				if data.AwardType == "gauntlet" && client.lastGauntletVictim != nil {
					awardEvent.VictimName = client.lastGauntletVictim.name
					if client.lastGauntletVictim.playerID > 0 {
						awardEvent.VictimPlayerID = &client.lastGauntletVictim.playerID
					}
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
			var playerID *int64
			if client, ok := state.clients[data.ClientID]; ok && client.playerID > 0 {
				playerID = &client.playerID
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventSay,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.SayEvent{
					ClientNum:  data.ClientID,
					PlayerName: data.Name,
					Message:    data.Message,
					PlayerID:   playerID,
				},
			})
		}

	case EventTypeSayTeam:
		data := event.Data.(SayTeamData)
		// Skip events in replay mode
		if !replayMode {
			var playerID *int64
			if client, ok := state.clients[data.ClientID]; ok && client.playerID > 0 {
				playerID = &client.playerID
			}
			m.emitEvent(domain.Event{
				Type:      domain.EventSayTeam,
				ServerID:  serverID,
				Timestamp: event.Timestamp,
				Data: domain.SayTeamEvent{
					ClientNum:  data.ClientID,
					PlayerName: data.Name,
					Message:    data.Message,
					PlayerID:   playerID,
				},
			})
		}

	case EventTypeTell:
		data := event.Data.(TellData)
		// Skip events in replay mode
		if !replayMode {
			var fromPlayerID, toPlayerID *int64
			if client, ok := state.clients[data.FromClientID]; ok && client.playerID > 0 {
				fromPlayerID = &client.playerID
			}
			if client, ok := state.clients[data.ToClientID]; ok && client.playerID > 0 {
				toPlayerID = &client.playerID
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
					FromPlayerID:  fromPlayerID,
					ToPlayerID:    toPlayerID,
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
		// Server startup without preceding shutdown indicates crash recovery
		// Close any open sessions that started before this timestamp
		if err := m.store.EndOpenSessionsBefore(ctx, serverID, event.Timestamp, event.Timestamp); err != nil {
			log.Printf("Error closing orphaned sessions on server startup: %v", err)
		}

	case EventTypeServerShutdown:
		// Clean server shutdown - close open sessions that started before this timestamp
		// (using timestamp filter to avoid closing sessions from later events during replay)
		if err := m.store.EndOpenSessionsBefore(ctx, serverID, event.Timestamp, event.Timestamp); err != nil {
			log.Printf("Error closing sessions on server shutdown: %v", err)
		}
	}
}

// handleMapChange handles a new map starting
func (m *ServerManager) handleMapChange(ctx context.Context, state *serverState, mapName string, gameType int, uuid string, ts time.Time, replayMode bool) {
	// Skip duplicate InitGame at same timestamp (Q3 sometimes logs it twice on server restart)
	if ts.Equal(state.lastInitGame) {
		return
	}
	state.lastInitGame = ts

	gameTypeStr := domain.GameTypeFromInt(gameType)

	// In replay mode, look up existing match from DB (never create new)
	if replayMode {
		prevMatch := state.match
		state.match = nil

		if existing, err := m.store.GetMatchByUUID(ctx, uuid); err == nil && existing != nil {
			state.match = existing
		}

		// If UUID lookup failed and we had a previous match with different UUID,
		// the previous match was interrupted (server crash). End it.
		if state.match == nil && prevMatch != nil && prevMatch.UUID != uuid && prevMatch.EndedAt == nil {
			m.store.EndMatch(ctx, prevMatch.ID, ts, "crashed", nil, nil)
		}

		// If no match found in DB, leave state.match = nil
		// Stats will be reconstructed at ShutdownGame when match is created
		state.clients = make(map[int]*clientState)
		state.matchState = ""
		state.warmupDuration = 0
		return
	}

	// Check if a match with this UUID already exists in the database
	// This handles the "late replay" case where an ongoing match's InitGame
	// is processed as live mode after a collector restart
	existing, err := m.store.GetMatchByUUID(ctx, uuid)
	if err != nil {
		log.Printf("Error checking for existing match UUID: %v", err)
	}

	if existing != nil {
		// Match already exists - this is a late replay, not a new match
		if existing.EndedAt == nil {
			// Match is still open - load it and continue
			log.Printf("Resuming existing open match UUID=%s on %s", uuid, mapName)
			state.match = existing

			// End any OTHER open matches (not this one) - they were interrupted
			if err := m.store.EndAllOpenMatches(ctx, state.server.ID, ts, "crashed", existing.ID); err != nil {
				log.Printf("Error ending other open matches: %v", err)
			}
		} else {
			// Match already ended - duplicate InitGame, load existing
			log.Printf("Match UUID=%s already ended, loading existing", uuid)
			state.match = existing
		}

		// Clear client state for state rebuild
		state.clients = make(map[int]*clientState)
		state.matchState = ""
		state.warmupDuration = 0
		return
	}

	// UUID is new - genuinely a new match (original behavior follows)
	if err := m.store.EndAllOpenMatches(ctx, state.server.ID, ts, "crashed", 0); err != nil {
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
	}
	state.match = match

	// Clear client state and reset match state for new map
	state.clients = make(map[int]*clientState)
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

// getMatchID returns the current match ID, looking up from DB if state.match.ID is 0 (placeholder from replay)
func (m *ServerManager) getMatchID(ctx context.Context, state *serverState) int64 {
	if state.match == nil {
		return 0
	}
	if state.match.ID > 0 {
		return state.match.ID
	}
	// Placeholder from replay - look up from DB by UUID first, then by server ID
	if state.match.UUID != "" {
		if match, err := m.store.GetMatchByUUID(ctx, state.match.UUID); err == nil && match != nil {
			state.match = match
			return match.ID
		}
	}
	// Fallback: look up by server ID
	if openMatch, err := m.store.GetCurrentMatch(ctx, state.server.ID); err == nil && openMatch != nil {
		state.match = openMatch
		return openMatch.ID
	}
	return 0
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

// computeMaxScore returns the maximum score among all clients (for FFA victory)
func computeMaxScore(clients map[int]*clientState) int {
	maxScore := 0
	for _, client := range clients {
		if client.playerGUID > 0 && client.score != nil && *client.score > maxScore {
			maxScore = *client.score
		}
	}
	return maxScore
}

// isMatchWinner determines if a client won the match based on game type and scores
func isMatchWinner(client *clientState, state *serverState, maxFFAScore int) bool {
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

	// FFA/Tournament: highest score wins (must be > 0)
	return maxFFAScore > 0 && client.score != nil && *client.score == maxFFAScore
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
	default:
		m.sendTell(serverID, clientID, "^1Unknown command: ^7"+cmd)
	}
}

// handleLinkCommand processes a link command from a player
func (m *ServerManager) handleLinkCommand(ctx context.Context, serverID int64, state *serverState, clientID int, args string) {
	client, ok := state.clients[clientID]
	if !ok {
		log.Printf("link: client %d not found in state", clientID)
		return
	}

	code := trimSpace(args)

	// Validate code format (6 digits)
	if len(code) != 6 || !isNumeric(code) {
		m.sendTell(serverID, clientID, "^3Usage: ^7!link <6-digit-code>")
		return
	}

	// Look up the link code
	linkCode, err := m.store.GetValidLinkCode(ctx, code)
	if err != nil {
		m.sendTell(serverID, clientID, "^1Invalid or expired link code.")
		return
	}

	// Get the primary player to compare names
	primaryPlayer, err := m.store.GetPlayerByID(ctx, linkCode.PlayerID)
	if err != nil {
		m.sendTell(serverID, clientID, "^1Error: Could not find primary player.")
		return
	}

	// Validate name match (exact clean_name match)
	if client.cleanName != primaryPlayer.CleanName {
		m.sendTell(serverID, clientID, "^1Name mismatch. ^7Your in-game name must match your primary player name.")
		return
	}

	// Check if this GUID already belongs to the primary player
	if client.playerID == linkCode.PlayerID {
		m.sendTell(serverID, clientID, "^3This GUID is already linked to your account.")
		return
	}

	// Check if the GUID has a valid player record
	if client.playerGUID == 0 || client.guid == "" {
		m.sendTell(serverID, clientID, "^1Error: Could not identify your GUID. Try reconnecting.")
		return
	}

	// Get the player record for this GUID (the source player to merge)
	sourcePlayerGUID, err := m.store.GetPlayerGUIDByGUID(ctx, client.guid)
	if err != nil || sourcePlayerGUID == nil {
		m.sendTell(serverID, clientID, "^1Error: Could not find player record for your GUID.")
		return
	}

	// Check if source and target are the same player (shouldn't happen given above check, but be safe)
	if sourcePlayerGUID.PlayerID == linkCode.PlayerID {
		m.sendTell(serverID, clientID, "^3This GUID is already linked to your account.")
		return
	}

	// Atomically: mark code as used, then merge
	if err := m.store.MarkLinkCodeUsed(ctx, linkCode.ID, client.guid); err != nil {
		m.sendTell(serverID, clientID, "^1Code already used or expired.")
		return
	}

	// Merge the source player (with this GUID) into the target primary player
	if err := m.store.MergePlayers(ctx, linkCode.PlayerID, sourcePlayerGUID.PlayerID); err != nil {
		log.Printf("Error merging players during link: %v", err)
		m.sendTell(serverID, clientID, "^1Error linking account. Please contact admin.")
		return
	}

	// Update client state to reflect new player_id
	client.playerID = linkCode.PlayerID

	m.sendTell(serverID, clientID, "^2Link successful! ^7Your GUID has been linked to your account.")
	log.Printf("Link successful: GUID %s merged into player %d via code %s", client.guid, linkCode.PlayerID, code)
}

// sendTell sends a private message to a player via RCON (runs async to avoid deadlock)
func (m *ServerManager) sendTell(serverID int64, clientID int, message string) {
	cmd := fmt.Sprintf("tell %d ^7%s", clientID, message)
	log.Printf("Sending RCON tell: %q", cmd)
	go func() {
		response, err := m.ExecuteRcon(serverID, cmd)
		if err != nil {
			log.Printf("Error sending tell to client %d on server %d: %v", clientID, serverID, err)
		} else {
			log.Printf("RCON response: %q", response)
		}
	}()
}

// greetPlayer sends a welcome message to a player when they join
func (m *ServerManager) greetPlayer(ctx context.Context, serverID int64, clientID int, playerID int64, playerName string) {
	// Get player stats
	stats, err := m.store.GetPlayerStatsByID(ctx, playerID, "all")
	if err != nil {
		log.Printf("Error getting stats for player %d: %v", playerID, err)
		return
	}

	// Check if player has linked account
	claimed, err := m.store.IsPlayerClaimed(ctx, playerID)
	if err != nil {
		log.Printf("Error checking if player %d is claimed: %v", playerID, err)
		return
	}

	var message string
	hasStats := stats.Stats.CompletedMatches > 0

	if claimed {
		if hasStats {
			message = fmt.Sprintf("Welcome back, %s^7! K/D: ^3%.2f ^7| Matches: ^3%d",
				playerName, stats.Stats.KDRatio, stats.Stats.CompletedMatches)
		} else {
			message = fmt.Sprintf("Welcome back, %s^7!", playerName)
		}
	} else {
		if hasStats {
			message = fmt.Sprintf("Welcome, %s^7! K/D: ^3%.2f ^7| Matches: ^3%d ^7- Visit ^5trinity.ernie.io ^7to link your account!",
				playerName, stats.Stats.KDRatio, stats.Stats.CompletedMatches)
		} else {
			message = fmt.Sprintf("Welcome, %s^7! Visit ^5trinity.ernie.io ^7to link your account!",
				playerName)
		}
	}

	m.sendTell(serverID, clientID, message)
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

// linkCodeCleanupLoop periodically removes expired link codes
func (m *ServerManager) linkCodeCleanupLoop(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if count, err := m.store.CleanupExpiredLinkCodes(ctx); err != nil {
				log.Printf("Error cleaning up expired link codes: %v", err)
			} else if count > 0 {
				log.Printf("Cleaned up %d expired link codes", count)
			}
		}
	}
}
