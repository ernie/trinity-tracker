package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/storage"
)

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes a JSON error response
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// parseID parses an ID from the URL path
func parseID(req *http.Request, param string) (int64, error) {
	idStr := req.PathValue(param)
	return strconv.ParseInt(idStr, 10, 64)
}

// Liveness thresholds for the live-cards endpoint.
//
// stalenessThreshold: a card with collector heartbeat older than this
// is rendered with a "stale" chip; the card still shows.
//
// hideThreshold: a card whose collector hasn't heartbeated OR whose
// last UDP success is older than this disappears from the response.
// The thresholds are intentionally distinct so a brief blip surfaces
// as a chip rather than a vanish.
const (
	livenessStalenessThreshold = 60 * time.Second
	livenessHideThreshold      = 5 * time.Minute
)

// liveServer is the live-cards payload: a server row plus the
// hub-derived liveness signal so the UI can render a "stale" or
// "offline" chip on the card itself. ManageableByMe is computed per
// request from auth claims + source ownership + admin delegation —
// the UI uses it to gate the click-to-open-RCON affordance.
type liveServer struct {
	domain.Server
	Online         bool   `json:"online"`
	Liveness       string `json:"liveness"` // "live" | "stale" | "offline"
	ManageableByMe bool   `json:"manageable_by_me"`
}

// handleGetServers returns servers the hub has observed enforcing
// g_trinityHandshake AND that are currently checking in. Servers go
// missing from this list when the collector source has stopped
// heartbeating OR the q3 server has been UDP-unreachable for the
// hide threshold — operators see them disappear instead of stuck on
// stale data.
func (r *Router) handleGetServers(w http.ResponseWriter, req *http.Request) {
	servers, err := r.store.GetServers(req.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Resolve auth once per request. owners is loaded lazily — anonymous
	// callers never get a non-zero manageable_by_me, so we skip the query.
	claims := r.getAuthClaims(req)
	var owners map[string]int64
	if claims != nil {
		owners, err = r.store.SourceOwners(req.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	now := time.Now().UTC()
	out := make([]liveServer, 0, len(servers))
	for _, s := range servers {
		if !s.HandshakeRequired {
			continue
		}
		// Heartbeat staleness.
		heartbeatAge := livenessHideThreshold + time.Second
		if s.LastHeartbeatAt != nil {
			heartbeatAge = now.Sub(*s.LastHeartbeatAt)
		}
		// UDP staleness — inferred from the poller's in-memory status.
		// status==nil happens before the first poll completes.
		status := r.lookupServerStatus(s.ID)
		online := false
		udpAge := livenessHideThreshold + time.Second
		if status != nil {
			online = status.Online
			if status.LastSeenAt != nil {
				udpAge = now.Sub(*status.LastSeenAt)
			}
		}
		if heartbeatAge > livenessHideThreshold || udpAge > livenessHideThreshold {
			continue
		}
		liveness := "live"
		switch {
		case !online:
			liveness = "offline"
		case heartbeatAge > livenessStalenessThreshold:
			liveness = "stale"
		}
		manageable := false
		if claims != nil {
			ownerID, hasOwner := owners[s.Source]
			isLocal := r.localSource != "" && s.Source == r.localSource
			switch {
			case hasOwner && ownerID == claims.UserID:
				// User owns this source — RCON regardless of admin status.
				manageable = true
			case claims.IsAdmin && isLocal:
				// Local hub+collector: admin runs the box.
				manageable = true
			case claims.IsAdmin && s.AdminDelegationEnabled:
				// Remote source whose operator opted in to admin delegation.
				// Includes admin-minted remotes (no owner row) — the
				// operator behind the .creds still has to flip the cfg
				// flag before admin gets RCON. The collector re-validates
				// on every proxy request as a backstop.
				manageable = true
			}
		}
		out = append(out, liveServer{Server: s, Online: online, Liveness: liveness, ManageableByMe: manageable})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetServer returns a single server
func (r *Router) handleGetServer(w http.ResponseWriter, req *http.Request) {
	id, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid server id")
		return
	}

	server, err := r.store.GetServerByID(req.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	writeJSON(w, http.StatusOK, server)
}

// handleGetServerStatus returns current status for a server
func (r *Router) handleGetServerStatus(w http.ResponseWriter, req *http.Request) {
	id, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid server id")
		return
	}

	status := r.lookupServerStatus(id)
	if status == nil {
		writeError(w, http.StatusNotFound, "server status not available")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// handleGetServerPlayers returns current players on a server
func (r *Router) handleGetServerPlayers(w http.ResponseWriter, req *http.Request) {
	id, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid server id")
		return
	}

	status := r.lookupServerStatus(id)
	if status == nil {
		writeError(w, http.StatusNotFound, "server status not available")
		return
	}

	// Response with player list and counts
	response := map[string]interface{}{
		"players":     status.Players,
		"human_count": status.HumanCount,
		"bot_count":   status.BotCount,
		"total":       len(status.Players),
	}
	writeJSON(w, http.StatusOK, response)
}

// handleGetPlayers returns players with optional search and pagination
func (r *Router) handleGetPlayers(w http.ResponseWriter, req *http.Request) {
	search := req.URL.Query().Get("search")
	if search != "" {
		limit := parseLimit(req, 20, 100)

		includeGUID := false
		if authHeader := req.Header.Get("Authorization"); authHeader != "" {
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				token := authHeader[7:]
				if _, err := r.auth.ValidateToken(token); err == nil {
					includeGUID = true
				}
			}
		}

		players, err := r.store.SearchPlayers(req.Context(), search, limit, includeGUID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, players)
		return
	}

	limit := parseLimit(req, 50, 100)
	offset := parseOffset(req)

	players, total, err := r.store.GetPlayers(req.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"players": players,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

// handleGetPlayer returns a single player
func (r *Router) handleGetPlayer(w http.ResponseWriter, req *http.Request) {
	id, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid player id")
		return
	}

	player, err := r.store.GetPlayerByID(req.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "player not found")
		return
	}
	writeJSON(w, http.StatusOK, player)
}

// handleGetPlayerStatsByID returns aggregated stats for a player by ID
func (r *Router) handleGetPlayerStatsByID(w http.ResponseWriter, req *http.Request) {
	id, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid player id")
		return
	}

	period := req.URL.Query().Get("period")
	if period == "" {
		period = "all"
	}

	if !validatePeriod(period) {
		writeError(w, http.StatusBadRequest, "invalid period: must be all, day, week, month, or year")
		return
	}

	stats, err := r.store.GetPlayerStatsByID(req.Context(), id, period)
	if err != nil {
		writeError(w, http.StatusNotFound, "player not found")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// handleGetSourceNames returns the list of source names + active flags
// from the sources table — public, used to populate the source-filter
// dropdown (which renders inactive sources with an "(inactive)" suffix
// so users can still filter historical matches by them).
// Admin-only /api/admin/sources also exists and returns the full
// per-source detail with credential download links.
func (r *Router) handleGetSourceNames(w http.ResponseWriter, req *http.Request) {
	rows, err := r.store.ListApprovedSources(req.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type entry struct {
		Source string `json:"source"`
		Active bool   `json:"active"`
	}
	out := make([]entry, 0, len(rows))
	for _, s := range rows {
		out = append(out, entry{Source: s.Source, Active: s.Active})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetMatches returns recent finished matches with server and player info
func (r *Router) handleGetMatches(w http.ResponseWriter, req *http.Request) {
	filter := storage.MatchFilter{
		Limit:    parseLimit(req, 20, 100),
		BeforeID: parseBeforeID(req),
	}

	// Game type filter
	if gt := req.URL.Query().Get("game_type"); gt != "" {
		if !validateGameType(gt) {
			writeError(w, http.StatusBadRequest, "invalid game_type")
			return
		}
		filter.GameType = gt
	}

	// Date filters (RFC3339 format)
	if sd := req.URL.Query().Get("start_date"); sd != "" {
		t, err := time.Parse(time.RFC3339, sd)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid start_date format, use RFC3339")
			return
		}
		filter.StartDate = &t
	}

	if ed := req.URL.Query().Get("end_date"); ed != "" {
		t, err := time.Parse(time.RFC3339, ed)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid end_date format, use RFC3339")
			return
		}
		filter.EndDate = &t
	}

	if src := req.URL.Query().Get("source"); src != "" {
		filter.Source = src
	}

	if mv := req.URL.Query().Get("movement"); mv != "" {
		if !validateMovementMode(mv) {
			writeError(w, http.StatusBadRequest, "invalid movement")
			return
		}
		filter.Movement = mv
	}

	if gp := req.URL.Query().Get("gameplay"); gp != "" {
		if !validateGameplayMode(gp) {
			writeError(w, http.StatusBadRequest, "invalid gameplay")
			return
		}
		filter.Gameplay = gp
	}

	filter.IncludeBotOnly = req.URL.Query().Get("include_bot_only") == "true"

	matches, err := r.store.GetFilteredMatchSummaries(req.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	r.populateDemoURLs(matches)
	writeJSON(w, http.StatusOK, matches)
}

// handleGetMatch returns a single match
func (r *Router) handleGetMatch(w http.ResponseWriter, req *http.Request) {
	id, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid match id")
		return
	}

	match, err := r.store.GetMatchSummaryByID(req.Context(), id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "match not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	matches := []domain.MatchSummary{*match}
	r.populateDemoURLs(matches)
	writeJSON(w, http.StatusOK, matches[0])
}

// handleGetLeaderboard returns top players by specified category and time period
func (r *Router) handleGetLeaderboard(w http.ResponseWriter, req *http.Request) {
	limit := parseLimit(req, 50, 100)

	category := req.URL.Query().Get("category")
	if category == "" {
		category = "frags"
	}
	if !validateCategory(category) {
		writeError(w, http.StatusBadRequest, "invalid category")
		return
	}

	period := req.URL.Query().Get("period")
	if period == "" {
		period = "all"
	}
	if !validatePeriod(period) {
		writeError(w, http.StatusBadRequest, "invalid period")
		return
	}

	gameType := req.URL.Query().Get("game_type")
	if gameType != "" && !validateGameType(gameType) {
		writeError(w, http.StatusBadRequest, "invalid game_type")
		return
	}

	response, err := r.store.GetLeaderboard(req.Context(), category, period, limit, gameType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// handleHealth returns a simple health check response
func (r *Router) handleHealth(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// handleMergePlayers merges another player into the target player
func (r *Router) handleMergePlayers(w http.ResponseWriter, req *http.Request) {
	targetID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid target player id")
		return
	}

	var body struct {
		MergePlayerID int64 `json:"merge_player_id"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.MergePlayerID == 0 {
		writeError(w, http.StatusBadRequest, "merge_player_id required")
		return
	}

	if body.MergePlayerID == targetID {
		writeError(w, http.StatusBadRequest, "cannot merge player into itself")
		return
	}

	if err := r.store.MergePlayers(req.Context(), targetID, body.MergePlayerID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Return the updated player
	player, err := r.store.GetPlayerByID(req.Context(), targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, player)
}

// handleSplitGUID splits a GUID into a new player
func (r *Router) handleSplitGUID(w http.ResponseWriter, req *http.Request) {
	guidID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid guid id")
		return
	}

	newPlayer, err := r.store.SplitGUID(req.Context(), guidID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, newPlayer)
}

// handleGetPlayerGUIDs returns all GUIDs for a player
func (r *Router) handleGetPlayerGUIDs(w http.ResponseWriter, req *http.Request) {
	playerID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid player id")
		return
	}

	guids, err := r.store.GetPlayerGUIDs(req.Context(), playerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, guids)
}

// handleGetPlayerMatches returns recent matches for a specific player
func (r *Router) handleGetPlayerMatches(w http.ResponseWriter, req *http.Request) {
	playerID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid player id")
		return
	}

	limit := parseLimit(req, 10, 50)
	beforeID := parseBeforeID(req)

	matches, err := r.store.GetPlayerRecentMatches(req.Context(), playerID, limit, beforeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	r.populateDemoURLs(matches)
	writeJSON(w, http.StatusOK, matches)
}

// handleGetPlayerSessions returns recent sessions for a specific player (admin only)
func (r *Router) handleGetPlayerSessions(w http.ResponseWriter, req *http.Request) {
	playerID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid player id")
		return
	}

	limit := parseLimit(req, 20, 100)
	beforeID := parseBeforeID(req)

	sessions, err := r.store.GetPlayerSessions(req.Context(), playerID, limit, beforeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, sessions)
}

// handleListAdminSessions returns recent sessions across all players,
// optionally filtered by server_id and/or player_id. Admin only.
func (r *Router) handleListAdminSessions(w http.ResponseWriter, req *http.Request) {
	var filter storage.SessionFilter

	if v := req.URL.Query().Get("server_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > 0 {
			filter.ServerID = &id
		}
	}
	if v := req.URL.Query().Get("player_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > 0 {
			filter.PlayerID = &id
		}
	}

	limit := parseLimit(req, 50, 200)
	beforeID := parseBeforeID(req)

	sessions, err := r.store.GetRecentSessions(req.Context(), filter, limit, beforeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, sessions)
}
