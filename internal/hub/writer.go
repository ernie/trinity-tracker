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

// notFound returns true when err signals "no rows" from the store. The
// lookup methods below normalize such errors to a nil-result/nil-error
// pair so callers can treat absence as a value rather than an error.
func notFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// Writer is the hub-side consumer of fact events. It owns the store
// and serializes all DB writes through a single consume goroutine.
//
// In standalone mode (M1) the collector's ServerManager pushes events
// into Publish(); in distributed mode (M2) a NATS subscription feeds
// the same channel. The collector code is identical across both modes
// because the public API is the same.
type Writer struct {
	store  *storage.Store
	events chan domain.FactEvent

	// preStop runs before the events channel is closed, giving an
	// embedded NATS server (or any other inbound source) a chance to
	// stop accepting work before the writer drains.
	preStop func()

	// publisher, when set, diverts Publish calls to a remote transport
	// instead of the in-process channel. Populated in distributed mode
	// (tracker.collector configured).
	publisher FactPublisher

	// localWatermark, when set, records the timestamp of each
	// successfully-dispatched event. Standalone mode uses this as its
	// replay cutoff on next boot; distributed mode leaves it nil and
	// relies on the natsbus watermark on the publisher side.
	localWatermark *LocalWatermarkTracker

	// presence tracks (serverID, clientNum) → guid for every
	// currently-connected human player, fed by handlePlayerJoin /
	// handlePlayerLeave. The hub poller uses it to enrich UDP
	// statusResponse rows with identity; EnrichEvent doesn't need it
	// (live events already carry GUID).
	presence *Presence

	// sources gates incoming fact-event envelopes by source_uuid.
	// Owned by the writer so HandleEnvelope can consult it without an
	// extra parameter.
	sources *SourceRegistry

	// guidCache memoizes GUID → player_id lookups for broadcast
	// enrichment. A resolved GUID is permanent until AssociateGUIDWithPlayer
	// (Trinity auth) or MergePlayers (!link) moves it; both paths
	// invalidate explicitly. Negative results are not cached — a GUID
	// may transition from unknown to known at any time via
	// UpsertPlayerIdentity, and re-querying the store on those
	// (rare-but-expected) misses is cheap.
	guidMu    sync.RWMutex
	guidCache map[string]int64

	wg       sync.WaitGroup
	stopOnce sync.Once
}

// Option configures a Writer at construction.
type Option func(*Writer)

// WithPreStop registers a function to run at the start of Stop, before
// the fact-event channel is closed. Used to shut down inbound sources
// (e.g. the embedded NATS server in hub mode) so the writer can drain
// cleanly.
func WithPreStop(fn func()) Option {
	return func(w *Writer) { w.preStop = fn }
}

// FactPublisher is the wire-side handoff used when the process has a
// collector role active under distributed tracking. The collector
// still calls Writer.Publish; the writer forwards to the publisher
// rather than dispatching to the local DB pipeline.
type FactPublisher interface {
	Publish(e domain.FactEvent) error
}

// WithFactPublisher swaps the writer's local Publish path for a
// remote publisher. When set, Writer.Publish serializes and forwards
// to NATS instead of dispatching in-process. The local dispatch
// goroutine still runs so hub-side consumption (phase 4+) can feed
// it from a NATS subscriber.
func WithFactPublisher(p FactPublisher) Option {
	return func(w *Writer) { w.publisher = p }
}

// WithLocalWatermark attaches a LocalWatermarkTracker to the writer so
// each successfully-dispatched event advances the on-disk watermark.
// Standalone mode uses this for replay cutoff on next boot.
func WithLocalWatermark(t *LocalWatermarkTracker) Option {
	return func(w *Writer) { w.localWatermark = t }
}

// eventBufferSize is the in-process fact-event channel capacity. Chosen
// larger than any plausible burst from a single match end (~32 players
// × a handful of events) to avoid blocking collectors in the hot path.
const eventBufferSize = 1024

// NewWriter constructs a Writer bound to the given store.
func NewWriter(store *storage.Store, opts ...Option) *Writer {
	w := &Writer{
		store:     store,
		events:    make(chan domain.FactEvent, eventBufferSize),
		guidCache: make(map[string]int64),
		sources:   NewSourceRegistry(store),
		presence:  NewPresence(),
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Presence exposes the hub's (serverID, clientNum) → GUID map so the
// poller can enrich UDP statusResponse rows with identity.
func (w *Writer) Presence() *Presence { return w.presence }

// ResolveIdentity satisfies IdentityResolver for the hub poller. It
// looks up the player_guids row for guid, then the verified/admin
// flags on the linked players row. ok is false when the GUID is
// unknown.
func (w *Writer) ResolveIdentity(ctx context.Context, guid string) (playerID int64, verified, admin, ok bool) {
	id, found := w.resolveGUIDPlayerID(ctx, guid)
	if !found {
		return 0, false, false, false
	}
	v, a := w.store.GetPlayerVerifiedStatus(ctx, id)
	return id, v, a, true
}

// MarkSourceApproved pre-approves a source_uuid for event dispatch.
// Used in hub+collector deployments to green-light the local
// collector's own source before any heartbeats have registered it.
func (w *Writer) MarkSourceApproved(sourceUUID string) {
	w.sources.MarkApproved(sourceUUID)
}

// ApproveSource is the admin flow: create/update servers rows for
// the pending source's roster (is_remote=1) and drain the in-memory
// DLQ through HandleEnvelope. The demoBaseURL is stamped on each
// row so Match links resolve to the remote collector's server.
func (w *Writer) ApproveSource(ctx context.Context, reg domain.Registration, demoBaseURL string) error {
	if err := w.store.ApproveRemoteServers(ctx, reg, demoBaseURL); err != nil {
		return err
	}
	w.sources.MarkApproved(reg.SourceUUID)
	for _, env := range w.sources.TakeDLQ(reg.SourceUUID) {
		if err := w.HandleEnvelope(ctx, env); err != nil {
			return err
		}
	}
	return nil
}

// RejectSource blocks a pending source and discards its DLQ.
func (w *Writer) RejectSource(sourceUUID string) error {
	if err := w.store.DeletePendingSource(context.Background(), sourceUUID); err != nil {
		return err
	}
	w.sources.Reject(sourceUUID)
	return nil
}

// resolveGUIDPlayerID returns the player_id for a GUID, hitting the
// in-memory cache first and falling through to the store on miss.
// Successful lookups are cached. Returns (0, false) if the GUID does
// not resolve.
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

// invalidateGUID drops a single GUID's cache entry. Called after an
// AssociateGUIDWithPlayer merge when Trinity auth links a GUID onto a
// user's player.
func (w *Writer) invalidateGUID(guid string) {
	if guid == "" {
		return
	}
	w.guidMu.Lock()
	delete(w.guidCache, guid)
	w.guidMu.Unlock()
}

// invalidateAllGUIDs flushes the entire cache. Called after MergePlayers
// (!link) since the mutation can touch every GUID attached to the
// source player and the writer doesn't track that membership.
func (w *Writer) invalidateAllGUIDs() {
	w.guidMu.Lock()
	w.guidCache = make(map[string]int64)
	w.guidMu.Unlock()
}

// Start launches the consume goroutine and any background maintenance
// loops. Runs until Stop is called or ctx is cancelled.
func (w *Writer) Start(ctx context.Context) {
	w.wg.Add(1)
	go w.run(ctx)
	w.wg.Add(1)
	go w.linkCodeCleanupLoop(ctx)
}

// Stop signals the consume goroutine to shut down and waits for it to
// drain. Any registered pre-stop hook runs first so inbound sources
// can quiesce before the channel closes. Safe to call more than once.
func (w *Writer) Stop() {
	w.stopOnce.Do(func() {
		if w.preStop != nil {
			w.preStop()
		}
		close(w.events)
	})
	w.wg.Wait()
}

// Publish sends a fact event to the writer. In standalone mode this
// blocks on the in-process channel if full — by design: losing fact
// events silently would corrupt the persistent state. In distributed
// mode (FactPublisher set) the event is forwarded to NATS and any
// publish error is returned. Writer.Publish satisfies FactPublisher.
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
		if w.localWatermark != nil {
			if err := w.localWatermark.Observe(e.Timestamp); err != nil {
				log.Printf("hub: local watermark observe: %v", err)
			}
		}
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
	case domain.PlayerLeaveData:
		w.handlePlayerLeave(ctx, e.ServerID, data)
	case domain.TrinityHandshakeData:
		w.handleTrinityHandshake(ctx, e.ServerID, data)
	case domain.ServerStartupData:
		w.handleServerStartup(ctx, e.ServerID, data)
	case domain.ServerShutdownData:
		w.handleServerShutdown(ctx, e.ServerID, data)
	default:
		log.Printf("hub.Writer: received %s event for server %d (dispatch not yet implemented)",
			e.Type, e.ServerID)
	}
}

// handleMatchStart persists a new match or adopts an existing row with
// the same UUID (idempotent — the collector may emit MatchStart for a
// match that was already created by a prior run's events).
//
// Matches from servers that don't require the Trinity handshake are
// refused. A well-behaved collector already skips match_start in that
// case; this is the safety net for anything that slips through. The
// gate scopes recorded stats to verified Trinity-client play (Q3
// servers don't run in pure mode, so g_trinityHandshake is the
// practical bar) and nudges operators toward the hub's account flow.
func (w *Writer) handleMatchStart(ctx context.Context, serverID int64, data domain.MatchStartData) {
	if !data.HandshakeRequired {
		log.Printf("hub: match_start rejected (server %d, uuid=%s): handshake_required=false", serverID, data.MatchUUID)
		return
	}
	if existing, err := w.store.GetMatchByUUID(ctx, data.MatchUUID); err != nil {
		log.Printf("hub: match_start UUID lookup failed: %v", err)
		return
	} else if existing != nil {
		// Already recorded. Fill in any columns that were empty (late replay fill).
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
// These are a single atomic operation as far as the collector is
// concerned: one MatchEndData event produces both writes. Every
// player with a resolvable GUID is flushed regardless of whether
// they've linked a users account — account linkage affects public
// leaderboards and greetings, not whether a match-end row exists.
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

// handlePlayerJoin creates a session row for a player's arrival. The
// collector emits this only for human players; bot identity is handled
// separately and does not generate sessions.
//
// Idempotent: if an open session already exists for this (server, guid)
// we leave it alone. This covers collector crash-replay, NATS retries,
// and map-change re-emission — any case where the collector is uncertain
// and publishes a belt-and-suspenders player_join.
func (w *Writer) handlePlayerJoin(ctx context.Context, serverID int64, data domain.PlayerJoinData) {
	// Record presence regardless of session outcome: a later poll for
	// server status should be able to identify the slot even when the
	// GUID is momentarily unknown to the store (late upsert arrival).
	w.presence.RecordJoin(serverID, data.ClientNum, data.GUID)

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

// handlePlayerLeave closes the open session for the given GUID on the
// given server.
func (w *Writer) handlePlayerLeave(ctx context.Context, serverID int64, data domain.PlayerLeaveData) {
	w.presence.RecordLeave(serverID, data.GUID)

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

// handleTrinityHandshake stamps the open session with client_engine /
// client_version.
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

// resolveOpenSession is a shared helper that resolves (serverID, GUID)
// to the matching open session row, or nil if none exists.
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

// PlayerIdentity is the result of an identity upsert or lookup. PlayerID
// is zero if the GUID is unknown. IsVerified/IsAdmin reflect the user
// account linked to the player (false if unlinked).
type PlayerIdentity struct {
	PlayerID     int64
	PlayerGUIDID int64
	IsVerified   bool
	IsAdmin      bool
	Found        bool
}

// UpsertPlayerIdentity upserts a human player_guid row and returns the
// canonical identity. Called by the collector at ClientUserinfo time.
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

// UpsertBotPlayerIdentity upserts a bot's synthetic player_guid row. Bots
// never have linked user accounts so verified/admin are always false.
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

// LookupPlayerIdentity reads an existing player_guid row without
// creating one. Used by the collector in replay mode. Returns Found=false
// if the GUID is unknown.
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

// Greet handles a greet RPC from the collector. It resolves identity,
// optionally verifies Trinity auth credentials (SipHash), auto-associates
// the GUID with the user's player on successful auth, and returns the
// data the collector needs to build the welcome rcon message.
//
// In distributed mode (M2) this becomes a NATS request/reply on
// trinity.rpc.greet.<source_id>. The reply shape is identical.
func (w *Writer) Greet(ctx context.Context, req GreetRequest) (GreetReply, error) {
	reply := GreetReply{AuthResult: AuthUnauthenticated}

	// Identity resolution: the collector has already upserted by the time
	// it calls Greet, so GetPlayerGUIDByGUID must find it.
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

	reply.PlayerID = playerID
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

// Claim handles a !claim chat-command RPC. Returns either a newly-minted
// claim code or a reason for skipping.
//
// PlayerID is optional on the wire — the collector no longer caches
// identity locally, so when it's zero we resolve from GUID.
func (w *Writer) Claim(ctx context.Context, req ClaimRequest) (ClaimReply, error) {
	playerID := req.PlayerID
	if playerID == 0 && req.GUID != "" {
		pg, err := w.store.GetPlayerGUIDByGUID(ctx, req.GUID)
		if notFound(err) || pg == nil {
			return ClaimReply{Status: ClaimUnknownPlayer}, nil
		}
		if err != nil {
			return ClaimReply{Status: ClaimError, Message: err.Error()}, nil
		}
		playerID = pg.PlayerID
	}
	if playerID == 0 {
		return ClaimReply{Status: ClaimUnknownPlayer}, nil
	}
	req.PlayerID = playerID
	claimed, err := w.store.IsPlayerClaimed(ctx, req.PlayerID)
	if err != nil {
		return ClaimReply{Status: ClaimError, Message: err.Error()}, nil
	}
	if claimed {
		return ClaimReply{Status: ClaimAlreadyClaimed}, nil
	}
	if err := w.store.InvalidatePlayerClaimCodes(ctx, req.PlayerID); err != nil {
		log.Printf("hub: claim invalidate existing codes for player %d: %v", req.PlayerID, err)
	}
	expires := time.Now().Add(30 * time.Minute)
	code, err := w.store.CreateClaimCode(ctx, req.PlayerID, expires)
	if err != nil {
		return ClaimReply{Status: ClaimError, Message: err.Error()}, nil
	}
	return ClaimReply{
		Status:    ClaimOK,
		Code:      code.Code,
		ExpiresAt: code.ExpiresAt,
	}, nil
}

// Link handles a !link <code> chat-command RPC. Validates the code and
// merges the GUID's player into the code's target player.
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
	// MergePlayers can have rewritten player_id for any GUID attached to
	// the source player. The writer doesn't track that list; flush the
	// whole cache rather than guess.
	w.invalidateAllGUIDs()
	return LinkReply{Status: LinkOK, NewPlayerID: linkCode.PlayerID}, nil
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// RegisterServer upserts a server row from config and returns the
// fully-populated domain.Server (including ID and last-match pointers).
// Called by the collector at startup for each configured Q3 server.
func (w *Writer) RegisterServer(ctx context.Context, name, address, logPath string) (*domain.Server, error) {
	dbSrv := &domain.Server{Name: name, Address: address, LogPath: logPath}
	if err := w.store.UpsertServer(ctx, dbSrv); err != nil {
		return nil, err
	}
	return w.store.GetServerByID(ctx, dbSrv.ID)
}

// TagLocalServerSource attaches (source_uuid, local_id) to an existing
// servers row so envelope handling can resolve RemoteServerID back to
// the hub's servers.id. Exposed so the NATS RPC RegisterServer handler
// can tag rows it just upserted without reaching around to the store.
func (w *Writer) TagLocalServerSource(ctx context.Context, serverID int64, sourceUUID string, localID int64) error {
	return w.store.TagLocalServerSource(ctx, serverID, sourceUUID, localID)
}

// linkCodeCleanupLoop periodically removes expired link codes. Hub-side
// maintenance task; moved here from the collector since link codes live
// in the hub's DB.
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

// EnrichEvent resolves the GUID fields on a broadcast event to their
// current player IDs via the store and returns an updated copy. Called
// by the router before forwarding to WebSocket clients.
//
// In distributed mode (M2) the collector has no DB so it cannot
// populate PlayerID fields itself; only the hub can. In standalone
// mode this same path runs and serves as the single source of truth
// for identity resolution on broadcasts.
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
		event.Data = d
	}
	return event
}
