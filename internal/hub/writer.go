package hub

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/storage"
)

func notFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// Writer is the hub-side consumer of fact events. It owns the store
// and serializes all DB writes through a single consume goroutine.
type Writer struct {
	store  *storage.Store
	events chan domain.FactEvent

	preStop func()

	publisher FactPublisher

	presence *Presence

	sources *SourceRegistry

	// guidCache memoizes GUID → player_id. Positive entries are
	// invalidated explicitly by AssociateGUIDWithPlayer and MergePlayers;
	// negative results are not cached because a GUID can transition to
	// known at any time via UpsertPlayerIdentity.
	guidMu    sync.RWMutex
	guidCache map[string]int64

	// handshakeState memoizes servers.handshake_required by serverID.
	// Updated authoritatively on every observed match_start (both accept
	// and reject paths). Lazy-loaded from the column on first lookup so
	// post-restart we don't have to warm the whole table.
	handshakeMu    sync.RWMutex
	handshakeState map[int64]bool

	wg       sync.WaitGroup
	stopOnce sync.Once
}

type Option func(*Writer)

// WithPreStop runs fn before Stop closes the event channel, so inbound
// sources (e.g. the embedded NATS server) can quiesce first.
func WithPreStop(fn func()) Option {
	return func(w *Writer) { w.preStop = fn }
}

// FactPublisher forwards fact events off-box instead of dispatching
// in-process.
type FactPublisher interface {
	Publish(e domain.FactEvent) error
}

// WithFactPublisher diverts Publish to a remote transport.
func WithFactPublisher(p FactPublisher) Option {
	return func(w *Writer) { w.publisher = p }
}

const eventBufferSize = 1024

func NewWriter(store *storage.Store, opts ...Option) *Writer {
	w := &Writer{
		store:          store,
		events:         make(chan domain.FactEvent, eventBufferSize),
		guidCache:      make(map[string]int64),
		handshakeState: make(map[int64]bool),
		sources:        NewSourceRegistry(store),
		presence:       NewPresence(),
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (w *Writer) Presence() *Presence { return w.presence }

// ResolveIdentity satisfies IdentityResolver for the hub poller.
func (w *Writer) ResolveIdentity(ctx context.Context, guid string) (playerID int64, verified, admin, ok bool) {
	id, found := w.resolveGUIDPlayerID(ctx, guid)
	if !found {
		return 0, false, false, false
	}
	v, a := w.store.GetPlayerVerifiedStatus(ctx, id)
	return id, v, a, true
}

// MarkSourceApproved primes the in-memory cache for a source known
// to exist. Callers must have already created the sources row (or
// know it was created elsewhere); this just skips the first DB
// lookup on the ingest path.
func (w *Writer) MarkSourceApproved(source string) {
	w.sources.MarkApproved(source)
}

// DeactivateSource marks the source + its servers inactive and flips
// the in-memory cache to Blocked. Rows stay in the DB so historical
// matches keep their (source, key) reference; the UI dims inactive
// content. Admin endpoint pairs this with creds revocation at the
// NATS layer.
func (w *Writer) DeactivateSource(ctx context.Context, source string) error {
	if err := w.store.DeactivateSource(ctx, source); err != nil {
		return err
	}
	w.sources.MarkBlocked(source)
	return nil
}

// ReactivateSource is the inverse of DeactivateSource.
func (w *Writer) ReactivateSource(ctx context.Context, source string) error {
	if err := w.store.ReactivateSource(ctx, source); err != nil {
		return err
	}
	w.sources.MarkApproved(source)
	return nil
}

// LeaveSource is the owner-self-service variant of DeactivateSource:
// flips status to 'left', marks the source blocked at ingest, and
// cascades servers inactive — same observable result as DeactivateSource
// except the row stays self-rejoinable. Caller pairs this with a
// userProv.RevokeSource() so any connected collector disconnects on
// next publish.
func (w *Writer) LeaveSource(ctx context.Context, source string) error {
	if err := w.store.LeaveSource(ctx, source); err != nil {
		return err
	}
	w.sources.MarkBlocked(source)
	return nil
}

func (w *Writer) resolveGUIDPlayerID(ctx context.Context, guid string) (int64, bool) {
	if guid == "" {
		return 0, false
	}
	w.guidMu.RLock()
	id, ok := w.guidCache[guid]
	w.guidMu.RUnlock()
	if ok {
		return id, true
	}
	pg, err := w.store.GetPlayerGUIDByGUID(ctx, guid)
	if err != nil || pg == nil {
		return 0, false
	}
	w.guidMu.Lock()
	w.guidCache[guid] = pg.PlayerID
	w.guidMu.Unlock()
	return pg.PlayerID, true
}

func (w *Writer) invalidateGUID(guid string) {
	if guid == "" {
		return
	}
	w.guidMu.Lock()
	delete(w.guidCache, guid)
	w.guidMu.Unlock()
}

func (w *Writer) invalidateAllGUIDs() {
	w.guidMu.Lock()
	w.guidCache = make(map[string]int64)
	w.guidMu.Unlock()
}

func (w *Writer) Start(ctx context.Context) {
	w.wg.Add(1)
	go w.run(ctx)
	w.wg.Add(1)
	go w.linkCodeCleanupLoop(ctx)
}

// Stop drains the consume goroutine. Safe to call more than once.
func (w *Writer) Stop() {
	w.stopOnce.Do(func() {
		if w.preStop != nil {
			w.preStop()
		}
		close(w.events)
	})
	w.wg.Wait()
}

// Publish forwards to the configured FactPublisher if set, otherwise
// blocks on the in-process channel (losing fact events would corrupt state).
func (w *Writer) Publish(e domain.FactEvent) error {
	if w.publisher != nil {
		return w.publisher.Publish(e)
	}
	w.events <- e
	return nil
}

func (w *Writer) run(ctx context.Context) {
	defer w.wg.Done()
	for e := range w.events {
		w.dispatch(ctx, e)
	}
}

func (w *Writer) dispatch(ctx context.Context, e domain.FactEvent) {
	switch data := e.Data.(type) {
	case domain.MatchStartData:
		w.handleMatchStart(ctx, e.ServerID, data)
	case domain.MatchEndData:
		w.handleMatchEnd(ctx, data)
	case domain.MatchSettingsUpdateData:
		w.handleMatchSettingsUpdate(ctx, data)
	case domain.MatchCrashedData:
		w.handleMatchCrashed(ctx, data)
	case domain.PlayerJoinData:
		w.handlePlayerJoin(ctx, e.ServerID, data)
	case domain.PresenceSnapshotData:
		w.handlePresenceSnapshot(e.ServerID, data)
	case domain.PlayerLeaveData:
		w.handlePlayerLeave(ctx, e.ServerID, data)
	case domain.TrinityHandshakeData:
		w.handleTrinityHandshake(ctx, e.ServerID, data)
	case domain.ServerStartupData:
		w.handleServerStartup(ctx, e.ServerID, data)
	case domain.ServerShutdownData:
		w.handleServerShutdown(ctx, e.ServerID, data)
	case domain.DemoFinalizedData:
		w.handleDemoFinalized(ctx, data)
	default:
		log.Printf("hub.Writer: received %s event for server %d (dispatch not yet implemented)",
			e.Type, e.ServerID)
	}
}

// IsHandshakeEnforced reports whether the most recently observed
// match_start for serverID had handshake_required=true. Returns false
// for servers we've never seen a match_start for. Backs the
// Router.Broadcast filter so live events for unproven servers don't
// reach the WebSocket.
func (w *Writer) IsHandshakeEnforced(ctx context.Context, serverID int64) bool {
	w.handshakeMu.RLock()
	v, ok := w.handshakeState[serverID]
	w.handshakeMu.RUnlock()
	if ok {
		return v
	}
	srv, err := w.store.GetServerByID(ctx, serverID)
	if err != nil || srv == nil {
		// Unknown server — refuse. This includes the FK-violation case
		// where a misrouted event references a serverID that doesn't
		// exist in the hub's table.
		return false
	}
	w.handshakeMu.Lock()
	w.handshakeState[serverID] = srv.HandshakeRequired
	w.handshakeMu.Unlock()
	return srv.HandshakeRequired
}

// recordHandshakeRequired persists the new value to the servers row and
// updates the in-memory cache. Cache update only happens on a successful
// DB write so the column stays authoritative — otherwise a transient
// UPDATE failure would leave the cache claiming "true" while the row
// stays 0, and a restart would silently flip the server out of the UI.
func (w *Writer) recordHandshakeRequired(ctx context.Context, serverID int64, required bool) {
	if err := w.store.SetServerHandshakeRequired(ctx, serverID, required); err != nil {
		log.Printf("hub: SetServerHandshakeRequired(server=%d, %v): %v", serverID, required, err)
		return
	}
	w.handshakeMu.Lock()
	w.handshakeState[serverID] = required
	w.handshakeMu.Unlock()
}

// handleMatchStart persists a new match or adopts an existing row with
// the same UUID (idempotent across collector replay and retries). Refuses
// matches from servers that don't require g_trinityHandshake so stats
// are scoped to clients that submit a Trinity handshake — independent
// of whether those handshakes carry valid auth info. Also latches the
// handshake state onto the servers row so the Router.Broadcast filter
// and the pollable-list filter know which servers to surface in the UI.
func (w *Writer) handleMatchStart(ctx context.Context, serverID int64, data domain.MatchStartData) {
	w.recordHandshakeRequired(ctx, serverID, data.HandshakeRequired)
	if !data.HandshakeRequired {
		log.Printf("hub: match_start rejected (server %d, uuid=%s): handshake_required=false", serverID, data.MatchUUID)
		return
	}
	w.presence.ResetCounters(serverID)
	if existing, err := w.store.GetMatchByUUID(ctx, data.MatchUUID); err != nil {
		log.Printf("hub: match_start UUID lookup failed: %v", err)
		return
	} else if existing != nil {
		if existing.Movement == "" && data.Movement != "" {
			if err := w.store.UpdateMatchMovement(ctx, existing.ID, data.Movement); err != nil {
				log.Printf("hub: update movement on existing match %d: %v", existing.ID, err)
			}
		}
		if existing.Gameplay == "" && data.Gameplay != "" {
			if err := w.store.UpdateMatchGameplay(ctx, existing.ID, data.Gameplay); err != nil {
				log.Printf("hub: update gameplay on existing match %d: %v", existing.ID, err)
			}
		}
		log.Printf("hub: match_start adopted existing match %d uuid=%s map=%s", existing.ID, data.MatchUUID, data.MapName)
		return
	}

	match := &domain.Match{
		UUID:      data.MatchUUID,
		ServerID:  serverID,
		MapName:   data.MapName,
		GameType:  data.GameType,
		StartedAt: data.StartedAt,
		Movement:  data.Movement,
		Gameplay:  data.Gameplay,
	}
	if err := w.store.CreateMatch(ctx, match); err != nil {
		log.Printf("hub: CreateMatch failed for UUID %s: %v", data.MatchUUID, err)
		return
	}
	log.Printf("hub: match_start created match %d uuid=%s server=%d map=%s", match.ID, data.MatchUUID, serverID, data.MapName)
}

// handleMatchEnd flushes per-player stats and closes the match row.
func (w *Writer) handleMatchEnd(ctx context.Context, data domain.MatchEndData) {
	match, err := w.store.GetMatchByUUID(ctx, data.MatchUUID)
	if err != nil {
		log.Printf("hub: match_end UUID lookup failed: %v", err)
		return
	}
	if match == nil {
		log.Printf("hub: match_end for unknown UUID %s; skipping", data.MatchUUID)
		return
	}

	flushed := 0
	for _, p := range data.Players {
		pg, err := w.store.GetPlayerGUIDByGUID(ctx, p.GUID)
		if err != nil || pg == nil {
			log.Printf("hub: match_end cannot resolve GUID %s: %v", p.GUID, err)
			continue
		}
		if err := w.store.FlushMatchPlayerStats(ctx, match.ID, pg.ID, p.ClientID,
			p.Frags, p.Deaths, p.Completed, p.Score, p.Team, p.Model, p.Skill, p.Victory,
			p.Captures, p.FlagReturns, p.Assists, p.Impressives, p.Excellents, p.Humiliations, p.Defends,
			p.IsBot, p.JoinedLate, p.JoinedAt, p.IsVR); err != nil {
			log.Printf("hub: FlushMatchPlayerStats for GUID %s: %v", p.GUID, err)
			continue
		}
		flushed++
	}

	if err := w.store.EndMatch(ctx, match.ID, data.EndedAt, data.ExitReason, data.RedScore, data.BlueScore); err != nil {
		log.Printf("hub: EndMatch failed for UUID %s: %v", data.MatchUUID, err)
		return
	}
	log.Printf("hub: match_end match=%d uuid=%s players=%d reason=%q", match.ID, data.MatchUUID, flushed, data.ExitReason)
}

func (w *Writer) handleMatchSettingsUpdate(ctx context.Context, data domain.MatchSettingsUpdateData) {
	match, err := w.store.GetMatchByUUID(ctx, data.MatchUUID)
	if err != nil || match == nil {
		if err != nil {
			log.Printf("hub: match_settings_update UUID lookup: %v", err)
		}
		return
	}
	if data.Movement != "" {
		if err := w.store.UpdateMatchMovement(ctx, match.ID, data.Movement); err != nil {
			log.Printf("hub: UpdateMatchMovement for UUID %s: %v", data.MatchUUID, err)
			return
		}
		log.Printf("hub: match_settings_update match=%d movement=%s", match.ID, data.Movement)
	}
	if data.Gameplay != "" {
		if err := w.store.UpdateMatchGameplay(ctx, match.ID, data.Gameplay); err != nil {
			log.Printf("hub: UpdateMatchGameplay for UUID %s: %v", data.MatchUUID, err)
			return
		}
		log.Printf("hub: match_settings_update match=%d gameplay=%s", match.ID, data.Gameplay)
	}
}

func (w *Writer) handleMatchCrashed(ctx context.Context, data domain.MatchCrashedData) {
	match, err := w.store.GetMatchByUUID(ctx, data.MatchUUID)
	if err != nil || match == nil {
		if err != nil {
			log.Printf("hub: match_crashed UUID lookup: %v", err)
		}
		return
	}
	if match.EndedAt != nil {
		return // already closed
	}
	if err := w.store.EndMatch(ctx, match.ID, data.EndedAt, "crashed", nil, nil); err != nil {
		log.Printf("hub: EndMatch (crashed) for UUID %s: %v", data.MatchUUID, err)
		return
	}
	log.Printf("hub: match_crashed match=%d uuid=%s ended_at=%s", match.ID, data.MatchUUID, data.EndedAt.Format(time.RFC3339))
}

// handlePresenceSnapshot restores an in-memory presence entry for a
// player the collector already has in state.clients — typically after
// a hub restart while a match was in progress. Unlike handlePlayerJoin
// it does no session work, because the player didn't actually join;
// they were already there and their DB session (if any) was persisted
// on the original join.
func (w *Writer) handlePresenceSnapshot(serverID int64, data domain.PresenceSnapshotData) {
	w.presence.RecordJoin(serverID, data.ClientNum, PresenceEntry{
		GUID:         data.GUID,
		Model:        data.Model,
		IsBot:        data.IsBot,
		IsVR:         data.IsVR,
		Skill:        data.Skill,
		Impressives:  data.Impressives,
		Excellents:   data.Excellents,
		Humiliations: data.Humiliations,
		Defends:      data.Defends,
		Captures:     data.Captures,
		Assists:      data.Assists,
	})
}

// handlePlayerJoin records presence and, for humans, opens a session.
// Idempotent: existing open sessions for (server, guid) are left alone.
func (w *Writer) handlePlayerJoin(ctx context.Context, serverID int64, data domain.PlayerJoinData) {
	w.presence.RecordJoin(serverID, data.ClientNum, PresenceEntry{
		GUID:  data.GUID,
		Model: data.Model,
		IsBot: data.IsBot,
		IsVR:  data.IsVR,
		Skill: data.Skill,
	})

	if data.IsBot {
		return
	}

	pg, err := w.store.GetPlayerGUIDByGUID(ctx, data.GUID)
	if notFound(err) || pg == nil {
		log.Printf("hub: player_join unknown GUID %s; skipping session create", data.GUID)
		return
	}
	if err != nil {
		log.Printf("hub: player_join GUID lookup %s: %v", data.GUID, err)
		return
	}
	if existing, err := w.store.GetOpenSessionForPlayer(ctx, pg.ID, serverID); err != nil && !notFound(err) {
		log.Printf("hub: player_join open-session check for GUID %s: %v", data.GUID, err)
		return
	} else if existing != nil {
		return
	}
	session := &domain.Session{
		PlayerGUIDID: pg.ID,
		ServerID:     serverID,
		JoinedAt:     data.JoinedAt,
		IPAddress:    data.IP,
	}
	if err := w.store.CreateSession(ctx, session); err != nil {
		log.Printf("hub: CreateSession for GUID %s: %v", data.GUID, err)
		return
	}
	log.Printf("hub: player_join session=%d guid=%s name=%s server=%d", session.ID, data.GUID, data.CleanName, serverID)
}

func (w *Writer) handlePlayerLeave(ctx context.Context, serverID int64, data domain.PlayerLeaveData) {
	w.presence.RecordLeave(serverID, data.ClientNum, data.GUID)

	session, err := w.resolveOpenSession(ctx, serverID, data.GUID)
	if err != nil {
		log.Printf("hub: player_leave resolve session: %v", err)
		return
	}
	if session == nil {
		log.Printf("hub: player_leave no open session for GUID %s on server %d", data.GUID, serverID)
		return
	}
	if err := w.store.EndSession(ctx, session.ID, data.LeftAt); err != nil {
		log.Printf("hub: EndSession for GUID %s: %v", data.GUID, err)
		return
	}
	log.Printf("hub: player_leave session=%d guid=%s duration=%ds", session.ID, data.GUID, data.DurationSeconds)
}

func (w *Writer) handleTrinityHandshake(ctx context.Context, serverID int64, data domain.TrinityHandshakeData) {
	session, err := w.resolveOpenSession(ctx, serverID, data.GUID)
	if err != nil {
		log.Printf("hub: trinity_handshake resolve session: %v", err)
		return
	}
	if session == nil {
		return
	}
	if err := w.store.UpdateSessionClientInfo(ctx, session.ID, data.ClientEngine, data.ClientVersion); err != nil {
		log.Printf("hub: UpdateSessionClientInfo for GUID %s: %v", data.GUID, err)
		return
	}
	log.Printf("hub: trinity_handshake session=%d guid=%s engine=%q version=%q", session.ID, data.GUID, data.ClientEngine, data.ClientVersion)
}

func (w *Writer) handleServerStartup(ctx context.Context, serverID int64, data domain.ServerStartupData) {
	w.presence.Clear(serverID)
	if err := w.store.EndOpenSessionsBefore(ctx, serverID, data.StartedAt, data.StartedAt); err != nil {
		log.Printf("hub: EndOpenSessionsBefore (startup) for server %d: %v", serverID, err)
		return
	}
	log.Printf("hub: server_startup server=%d swept open sessions before %s", serverID, data.StartedAt.Format(time.RFC3339))
}

func (w *Writer) handleServerShutdown(ctx context.Context, serverID int64, data domain.ServerShutdownData) {
	w.presence.Clear(serverID)
	if err := w.store.EndOpenSessionsBefore(ctx, serverID, data.ShutdownAt, data.ShutdownAt); err != nil {
		log.Printf("hub: EndOpenSessionsBefore (shutdown) for server %d: %v", serverID, err)
		return
	}
	log.Printf("hub: server_shutdown server=%d swept open sessions before %s", serverID, data.ShutdownAt.Format(time.RFC3339))
}

// handleDemoFinalized flips matches.demo_available so the UI knows to
// render a play button. Idempotent — if the match doesn't exist or is
// already flagged, this is a no-op (the UPDATE just affects 0 rows).
func (w *Writer) handleDemoFinalized(ctx context.Context, data domain.DemoFinalizedData) {
	if data.MatchUUID == "" {
		return
	}
	if err := w.store.MarkMatchDemoAvailable(ctx, data.MatchUUID); err != nil {
		log.Printf("hub: MarkMatchDemoAvailable(%s): %v", data.MatchUUID, err)
		return
	}
	log.Printf("hub: demo_finalized match=%s frames=%d duration_ms=%d bytes=%d",
		data.MatchUUID, data.Frames, data.DurationMS, data.Bytes)
}

func (w *Writer) resolveOpenSession(ctx context.Context, serverID int64, guid string) (*domain.Session, error) {
	pg, err := w.store.GetPlayerGUIDByGUID(ctx, guid)
	if notFound(err) || pg == nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	session, err := w.store.GetOpenSessionForPlayer(ctx, pg.ID, serverID)
	if notFound(err) {
		return nil, nil
	}
	return session, err
}

// PlayerIdentity is the result of an identity upsert or lookup.
type PlayerIdentity struct {
	PlayerID     int64
	PlayerGUIDID int64
	IsVerified   bool
	IsAdmin      bool
	Found        bool
}

func (w *Writer) UpsertPlayerIdentity(ctx context.Context, guid, name, cleanName string, ts time.Time, isVR bool) (PlayerIdentity, error) {
	pg, err := w.store.UpsertPlayerGUID(ctx, guid, name, cleanName, ts, isVR)
	if err != nil || pg == nil {
		return PlayerIdentity{}, err
	}
	verified, admin := w.store.GetPlayerVerifiedStatus(ctx, pg.PlayerID)
	return PlayerIdentity{
		PlayerID:     pg.PlayerID,
		PlayerGUIDID: pg.ID,
		IsVerified:   verified,
		IsAdmin:      admin,
		Found:        true,
	}, nil
}

func (w *Writer) UpsertBotPlayerIdentity(ctx context.Context, name, cleanName string, ts time.Time) (PlayerIdentity, error) {
	pg, err := w.store.UpsertBotPlayerGUID(ctx, name, cleanName, ts)
	if err != nil || pg == nil {
		return PlayerIdentity{}, err
	}
	return PlayerIdentity{
		PlayerID:     pg.PlayerID,
		PlayerGUIDID: pg.ID,
		Found:        true,
	}, nil
}

// LookupPlayerIdentity reads an existing player_guid row without creating one.
func (w *Writer) LookupPlayerIdentity(ctx context.Context, guid string) (PlayerIdentity, error) {
	pg, err := w.store.GetPlayerGUIDByGUID(ctx, guid)
	if notFound(err) || pg == nil {
		return PlayerIdentity{}, nil
	}
	if err != nil {
		return PlayerIdentity{}, err
	}
	verified, admin := w.store.GetPlayerVerifiedStatus(ctx, pg.PlayerID)
	return PlayerIdentity{
		PlayerID:     pg.PlayerID,
		PlayerGUIDID: pg.ID,
		IsVerified:   verified,
		IsAdmin:      admin,
		Found:        true,
	}, nil
}

// Greet resolves identity, optionally verifies Trinity auth credentials
// via SipHash, and auto-associates the GUID with the user's player on
// successful auth.
func (w *Writer) Greet(ctx context.Context, req GreetRequest) (GreetReply, error) {
	reply := GreetReply{AuthResult: AuthUnauthenticated}

	pg, err := w.store.GetPlayerGUIDByGUID(ctx, req.GUID)
	if err != nil {
		return reply, err
	}
	if pg == nil {
		return reply, nil
	}
	playerID := pg.PlayerID

	if req.Auth != nil {
		authPlayerID, token, authErr := w.store.GetGameTokenByUsername(ctx, req.Auth.Username)
		switch {
		case authErr != nil:
			log.Printf("hub: greet auth no token for %q: %v", req.Auth.Username, authErr)
			reply.AuthResult = AuthFailed
		case sipHashHex(token, req.Auth.Nonce) != req.Auth.TokenHash:
			log.Printf("hub: greet auth hash mismatch for user %q", req.Auth.Username)
			reply.AuthResult = AuthFailed
		default:
			reply.AuthResult = AuthVerified
			merged, mergeErr := w.store.AssociateGUIDWithPlayer(ctx, req.GUID, authPlayerID)
			if mergeErr != nil {
				log.Printf("hub: greet associate GUID %q with player %d: %v", req.GUID, authPlayerID, mergeErr)
			} else if merged {
				reply.GUIDLinked = true
				w.invalidateGUID(req.GUID)
			}
			playerID = authPlayerID
		}
	}

	reply.CanonicalName = pg.CleanName

	stats, err := w.store.GetPlayerStatsByID(ctx, playerID, "all")
	if err == nil && stats != nil {
		reply.KDRatio = stats.Stats.KDRatio
		reply.CompletedMatches = stats.Stats.CompletedMatches
	}

	claimed, err := w.store.IsPlayerClaimed(ctx, playerID)
	if err == nil {
		reply.Claimed = claimed
	}

	verified, admin := w.store.GetPlayerVerifiedStatus(ctx, playerID)
	reply.IsVerified = verified
	reply.IsAdmin = admin

	return reply, nil
}

// Claim handles a !claim chat-command RPC.
func (w *Writer) Claim(ctx context.Context, req ClaimRequest) (ClaimReply, error) {
	if req.GUID == "" {
		return ClaimReply{Status: ClaimUnknownPlayer}, nil
	}
	pg, err := w.store.GetPlayerGUIDByGUID(ctx, req.GUID)
	if notFound(err) || pg == nil {
		return ClaimReply{Status: ClaimUnknownPlayer}, nil
	}
	if err != nil {
		return ClaimReply{Status: ClaimError, Message: err.Error()}, nil
	}
	playerID := pg.PlayerID
	claimed, err := w.store.IsPlayerClaimed(ctx, playerID)
	if err != nil {
		return ClaimReply{Status: ClaimError, Message: err.Error()}, nil
	}
	if claimed {
		return ClaimReply{Status: ClaimAlreadyClaimed}, nil
	}
	if err := w.store.InvalidatePlayerClaimCodes(ctx, playerID); err != nil {
		log.Printf("hub: claim invalidate existing codes for player %d: %v", playerID, err)
	}
	expires := time.Now().Add(30 * time.Minute)
	code, err := w.store.CreateClaimCode(ctx, playerID, expires)
	if err != nil {
		return ClaimReply{Status: ClaimError, Message: err.Error()}, nil
	}
	return ClaimReply{
		Status:    ClaimOK,
		Code:      code.Code,
		ExpiresAt: code.ExpiresAt,
	}, nil
}

// Link handles a !link <code> chat-command RPC.
func (w *Writer) Link(ctx context.Context, req LinkRequest) (LinkReply, error) {
	if len(req.Code) != 6 || !isNumeric(req.Code) {
		return LinkReply{Status: LinkInvalidFormat}, nil
	}
	linkCode, err := w.store.GetValidLinkCode(ctx, req.Code)
	if err != nil || linkCode == nil {
		return LinkReply{Status: LinkInvalidCode}, nil
	}
	sourcePG, err := w.store.GetPlayerGUIDByGUID(ctx, req.GUID)
	if err != nil || sourcePG == nil {
		return LinkReply{Status: LinkUnknownGUID}, nil
	}
	if sourcePG.PlayerID == linkCode.PlayerID {
		return LinkReply{Status: LinkAlreadyLinked}, nil
	}
	if err := w.store.MarkLinkCodeUsed(ctx, linkCode.ID, req.GUID); err != nil {
		return LinkReply{Status: LinkInvalidCode, Message: err.Error()}, nil
	}
	if err := w.store.MergePlayers(ctx, linkCode.PlayerID, sourcePG.PlayerID); err != nil {
		return LinkReply{Status: LinkError, Message: err.Error()}, nil
	}
	w.invalidateAllGUIDs()
	return LinkReply{Status: LinkOK}, nil
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (w *Writer) RegisterServer(ctx context.Context, source, key, address string) (*domain.Server, error) {
	dbSrv := &domain.Server{Key: key, Address: address}
	if err := w.store.UpsertServer(ctx, source, dbSrv); err != nil {
		return nil, err
	}
	return w.store.GetServerByID(ctx, dbSrv.ID)
}

// TagLocalServerSource attaches (source, local_id) so envelope
// handling can resolve RemoteServerID back to servers.id.
func (w *Writer) TagLocalServerSource(ctx context.Context, serverID int64, source string, localID int64) error {
	return w.store.TagLocalServerSource(ctx, serverID, source, localID)
}

func (w *Writer) linkCodeCleanupLoop(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := w.store.CleanupExpiredLinkCodes(ctx)
			if err != nil {
				log.Printf("hub: cleanup expired link codes: %v", err)
			} else if count > 0 {
				log.Printf("hub: cleaned up %d expired link codes", count)
			}
		}
	}
}

// EnrichEvent resolves GUID fields on a broadcast event to player IDs
// and bumps the presence award counters that drive the live cards.
func (w *Writer) EnrichEvent(ctx context.Context, event domain.Event) domain.Event {
	resolve := func(guid string) *int64 {
		id, ok := w.resolveGUIDPlayerID(ctx, guid)
		if !ok {
			return nil
		}
		return &id
	}

	switch d := event.Data.(type) {
	case domain.PlayerJoinEvent:
		if id := resolve(d.Player.GUID); id != nil {
			d.Player.PlayerID = id
			d.PlayerID = id
			verified, admin := w.store.GetPlayerVerifiedStatus(ctx, *id)
			d.Player.IsVerified = verified
			d.Player.IsAdmin = admin
		}
		event.Data = d
	case domain.PlayerLeaveEvent:
		d.PlayerID = resolve(d.GUID)
		event.Data = d
	case domain.FragEvent:
		d.FraggerPlayerID = resolve(d.FraggerGUID)
		d.VictimPlayerID = resolve(d.VictimGUID)
		event.Data = d
	case domain.FlagCaptureEvent:
		d.PlayerID = resolve(d.GUID)
		w.presence.IncrementByGUID(event.ServerID, d.GUID, AwardCapture)
		event.Data = d
	case domain.FlagTakenEvent:
		d.PlayerID = resolve(d.GUID)
		event.Data = d
	case domain.FlagReturnEvent:
		d.PlayerID = resolve(d.GUID)
		event.Data = d
	case domain.FlagDropEvent:
		d.PlayerID = resolve(d.GUID)
		event.Data = d
	case domain.ObeliskDestroyEvent:
		d.PlayerID = resolve(d.GUID)
		event.Data = d
	case domain.SkullScoreEvent:
		d.PlayerID = resolve(d.GUID)
		event.Data = d
	case domain.TeamChangeEvent:
		d.PlayerID = resolve(d.GUID)
		event.Data = d
	case domain.SayEvent:
		d.PlayerID = resolve(d.GUID)
		event.Data = d
	case domain.SayTeamEvent:
		d.PlayerID = resolve(d.GUID)
		event.Data = d
	case domain.TellEvent:
		d.FromPlayerID = resolve(d.FromGUID)
		d.ToPlayerID = resolve(d.ToGUID)
		event.Data = d
	case domain.AwardEvent:
		d.PlayerID = resolve(d.GUID)
		d.VictimPlayerID = resolve(d.VictimGUID)
		switch d.AwardType {
		case "impressive":
			w.presence.IncrementByGUID(event.ServerID, d.GUID, AwardImpressive)
		case "excellent":
			w.presence.IncrementByGUID(event.ServerID, d.GUID, AwardExcellent)
		case "humiliation":
			w.presence.IncrementByGUID(event.ServerID, d.GUID, AwardHumiliation)
		case "defend":
			w.presence.IncrementByGUID(event.ServerID, d.GUID, AwardDefend)
		case "assist":
			w.presence.IncrementByGUID(event.ServerID, d.GUID, AwardAssist)
		}
		event.Data = d
	case domain.ClientUserinfoEvent:
		w.presence.UpdateClientUserinfo(event.ServerID, d.ClientNum, d.GUID, d.Model, d.IsVR)
		event.Data = d
	}
	return event
}
