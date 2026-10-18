package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/storage"
)

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError sends {"error": message}; the web UI shows message as is.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func parseID(req *http.Request, param string) (int64, error) {
	idStr := req.PathValue(param)
	return strconv.ParseInt(idStr, 10, 64)
}

// Live-card liveness: a heartbeat older than the staleness threshold
// shows a stale chip, and a card hides once its heartbeat or its last
// UDP reply is older than the hide threshold. The gap between them
// lets a brief blip show as a chip.
const (
	livenessStalenessThreshold = 60 * time.Second
	livenessHideThreshold      = 5 * time.Minute
)

// liveServer is a live card: the server row plus its liveness, and
// whether the caller may use RCON on it.
type liveServer struct {
	domain.Server
	Online         bool   `json:"online"`
	Liveness       string `json:"liveness"` // "live" | "stale" | "offline"
	ManageableByMe bool   `json:"manageable_by_me"`
}

// handleGetVersion returns the running binary's version, or "dev".
func (r *Router) handleGetVersion(w http.ResponseWriter, req *http.Request) {
	v := r.version
	if v == "" {
		v = "dev"
	}
	writeJSON(w, http.StatusOK, map[string]string{"version": v})
}

// handleGetServers returns the live cards: servers seen enforcing
// g_trinityHandshake whose collector and game server are both
// answering.
func (r *Router) handleGetServers(w http.ResponseWriter, req *http.Request) {
	servers, err := r.store.GetServers(req.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Anonymous callers can't manage anything, so they skip the owner query.
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
		heartbeatAge := livenessHideThreshold + time.Second
		if s.LastHeartbeatAt != nil {
			heartbeatAge = now.Sub(*s.LastHeartbeatAt)
		}
		// Status is nil until the first poll completes.
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
				manageable = true
			case claims.IsAdmin && isLocal:
				// Admins run the hub's own collector.
				manageable = true
			case claims.IsAdmin && s.AdminDelegationEnabled:
				// The operator opted in; the collector checks again on
				// every request.
				manageable = true
			}
		}
		out = append(out, liveServer{Server: s, Online: online, Liveness: liveness, ManageableByMe: manageable})
	}
	writeJSON(w, http.StatusOK, out)
}

func (r *Router) handleGetServer(w http.ResponseWriter, req *http.Request) {
	id, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid server ID.")
		return
	}

	server, err := r.store.GetServerByID(req.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Server not found.")
		return
	}
	writeJSON(w, http.StatusOK, server)
}

func (r *Router) handleGetServerStatus(w http.ResponseWriter, req *http.Request) {
	id, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid server ID.")
		return
	}

	status := r.lookupServerStatus(id)
	if status == nil {
		writeError(w, http.StatusNotFound, "Server status isn't available.")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (r *Router) handleGetServerPlayers(w http.ResponseWriter, req *http.Request) {
	id, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid server ID.")
		return
	}

	status := r.lookupServerStatus(id)
	if status == nil {
		writeError(w, http.StatusNotFound, "Server status isn't available.")
		return
	}

	response := map[string]interface{}{
		"players":     status.Players,
		"human_count": status.HumanCount,
		"bot_count":   status.BotCount,
		"total":       len(status.Players),
	}
	writeJSON(w, http.StatusOK, response)
}

// handleGetPlayers searches players when search is set (matching GUIDs
// too for a valid bearer token), and otherwise lists them a page at a
// time.
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

	opts := storage.PlayerListOptions{
		Limit:       parseLimit(req, 50, 100),
		Offset:      parseOffset(req),
		Sort:        "last_seen",
		IncludeBots: req.URL.Query().Get("include_bots") == "true",
	}
	if v := req.URL.Query().Get("sort"); v != "" {
		if !storage.ValidPlayerSort(v) {
			writeError(w, http.StatusBadRequest, "Invalid sort.")
			return
		}
		opts.Sort = v
	}
	switch req.URL.Query().Get("dir") {
	case "":
		opts.Desc = opts.Sort != "name" // recency/playtime read best newest-first
	case "asc":
	case "desc":
		opts.Desc = true
	default:
		writeError(w, http.StatusBadRequest, "Invalid sort direction.")
		return
	}

	players, total, err := r.store.GetPlayers(req.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"players": players,
		"total":   total,
		"limit":   opts.Limit,
		"offset":  opts.Offset,
	})
}

func (r *Router) handleGetPlayer(w http.ResponseWriter, req *http.Request) {
	id, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid player ID.")
		return
	}

	player, err := r.store.GetPlayerByID(req.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Player not found.")
		return
	}
	writeJSON(w, http.StatusOK, player)
}

func (r *Router) handleGetPlayerStatsByID(w http.ResponseWriter, req *http.Request) {
	id, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid player ID.")
		return
	}

	period := req.URL.Query().Get("period")
	if period == "" {
		period = "all"
	}

	if !validatePeriod(period) {
		writeError(w, http.StatusBadRequest, "Invalid period. Use all, day, week, month or year.")
		return
	}

	stats, err := r.store.GetPlayerStatsByID(req.Context(), id, period)
	if err != nil {
		writeError(w, http.StatusNotFound, "Player not found.")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// handleGetSourceNames returns each source's name and active flag,
// inactive ones included so older matches can still be filtered by them.
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

// handleGetMatches returns finished matches, newest first, with their
// server and players.
func (r *Router) handleGetMatches(w http.ResponseWriter, req *http.Request) {
	filter := storage.MatchFilter{
		Limit:    parseLimit(req, 20, 100),
		BeforeID: parseBeforeID(req),
	}

	if gt := req.URL.Query().Get("game_type"); gt != "" {
		if !validateGameType(gt) {
			writeError(w, http.StatusBadRequest, "Invalid game_type.")
			return
		}
		filter.GameType = gt
	}

	if sd := req.URL.Query().Get("start_date"); sd != "" {
		t, err := time.Parse(time.RFC3339, sd)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid start_date. Use RFC 3339.")
			return
		}
		filter.StartDate = &t
	}

	if ed := req.URL.Query().Get("end_date"); ed != "" {
		t, err := time.Parse(time.RFC3339, ed)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid end_date. Use RFC 3339.")
			return
		}
		filter.EndDate = &t
	}

	if src := req.URL.Query().Get("source"); src != "" {
		filter.Source = src
	}

	if mode := req.URL.Query().Get("mode"); mode != "" {
		if !validateMode(mode) {
			writeError(w, http.StatusBadRequest, "Invalid mode.")
			return
		}
		filter.Mode = mode
	}

	filter.IncludeBotOnly = req.URL.Query().Get("include_bot_only") == "true"
	// featured=true ignores demo state so admins can find featured
	// matches whose demo is gone.
	filter.FeaturedOnly = req.URL.Query().Get("featured") == "true"
	filter.DemoOnly = req.URL.Query().Get("has_demo") == "true"

	matches, err := r.store.GetFilteredMatchSummaries(req.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	r.populateDemoURLs(matches)
	writeJSON(w, http.StatusOK, matches)
}

func (r *Router) handleGetMatch(w http.ResponseWriter, req *http.Request) {
	id, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid match ID.")
		return
	}

	match, err := r.store.GetMatchSummaryByID(req.Context(), id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "Match not found.")
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

// handleGetLeaderboard returns the top players for a category over a
// period, optionally limited to one game type.
func (r *Router) handleGetLeaderboard(w http.ResponseWriter, req *http.Request) {
	limit := parseLimit(req, 50, 100)

	category := req.URL.Query().Get("category")
	if category == "" {
		category = "frags"
	}
	if !validateCategory(category) {
		writeError(w, http.StatusBadRequest, "Invalid category.")
		return
	}

	period := req.URL.Query().Get("period")
	if period == "" {
		period = "all"
	}
	if !validatePeriod(period) {
		writeError(w, http.StatusBadRequest, "Invalid period.")
		return
	}

	gameType := req.URL.Query().Get("game_type")
	if gameType != "" && !validateGameType(gameType) {
		writeError(w, http.StatusBadRequest, "Invalid game_type.")
		return
	}

	// as_of pins the period's end so a snapshot link, such as the Discord
	// digest's, always shows the same board.
	var asOf time.Time
	if s := req.URL.Query().Get("as_of"); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid as_of: "+err.Error())
			return
		}
		asOf = parsed
	}

	response, err := r.store.GetLeaderboard(req.Context(), category, period, limit, gameType, asOf)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (r *Router) handleHealth(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// handleMergePlayers merges merge_player_id into the player in the path
// and returns the merged player.
func (r *Router) handleMergePlayers(w http.ResponseWriter, req *http.Request) {
	targetID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid target player ID.")
		return
	}

	var body struct {
		MergePlayerID int64 `json:"merge_player_id"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}

	if body.MergePlayerID == 0 {
		writeError(w, http.StatusBadRequest, "merge_player_id is required.")
		return
	}

	if body.MergePlayerID == targetID {
		writeError(w, http.StatusBadRequest, "A player can't be merged into itself.")
		return
	}

	if err := r.store.MergePlayers(req.Context(), targetID, body.MergePlayerID); err != nil {
		if errors.Is(err, storage.ErrBothPlayersLinked) {
			writeError(w, http.StatusConflict, "Both players are linked to accounts. Unlink one under Users first.")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	player, err := r.store.GetPlayerByID(req.Context(), targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, player)
}

// handleSplitGUID moves a GUID off its player onto a new player.
func (r *Router) handleSplitGUID(w http.ResponseWriter, req *http.Request) {
	guidID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid GUID ID.")
		return
	}

	newPlayer, err := r.store.SplitGUID(req.Context(), guidID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, newPlayer)
}

func (r *Router) handleGetPlayerGUIDs(w http.ResponseWriter, req *http.Request) {
	playerID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid player ID.")
		return
	}

	guids, err := r.store.GetPlayerGUIDs(req.Context(), playerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, guids)
}

func (r *Router) handleGetPlayerMatches(w http.ResponseWriter, req *http.Request) {
	playerID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid player ID.")
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

func (r *Router) handleGetPlayerSessions(w http.ResponseWriter, req *http.Request) {
	playerID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid player ID.")
		return
	}

	limit := parseLimit(req, 20, 100)
	beforeID := parseBeforeID(req)

	sessions, err := r.store.GetPlayerSessions(req.Context(), playerID, limit, beforeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// A nil slice would encode as null; clients expect an array.
	if sessions == nil {
		sessions = []domain.PlayerSession{}
	}

	writeJSON(w, http.StatusOK, sessions)
}

// handleListAdminSessions returns recent sessions across all players,
// optionally filtered by server_id, player_id and a since/until range
// on join time.
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
	// Malformed bounds are ignored so a stale saved filter can't break
	// the page.
	if v := req.URL.Query().Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.Since = &t
		}
	}
	if v := req.URL.Query().Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.Until = &t
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

// handlePlatformStats takes optional RFC 3339 since/until bounds on
// join time.
func (r *Router) handlePlatformStats(w http.ResponseWriter, req *http.Request) {
	var since, until *time.Time
	if v := req.URL.Query().Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			since = &t
		}
	}
	if v := req.URL.Query().Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			until = &t
		}
	}
	stats, err := r.store.GetPlatformStats(req.Context(), since, until)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// handleFeatureMatch features a match on POST and unfeatures it on
// DELETE.
func (r *Router) handleFeatureMatch(w http.ResponseWriter, req *http.Request) {
	idStr := req.PathValue("id")
	matchID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid match ID.", http.StatusBadRequest)
		return
	}

	featured := req.Method == http.MethodPost
	if err := r.store.SetMatchFeatured(req.Context(), matchID, featured); err != nil {
		http.Error(w, "Internal error.", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetFeaturedMatches returns the IDs of featured matches that have
// a demo, for the landing page's Watch a fight door.
func (r *Router) handleGetFeaturedMatches(w http.ResponseWriter, req *http.Request) {
	limit := 20
	if v := req.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	ids, err := r.store.GetFeaturedMatches(req.Context(), limit)
	if err != nil {
		http.Error(w, "Internal error.", http.StatusInternalServerError)
		return
	}
	if ids == nil {
		ids = []int64{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=30")
	json.NewEncoder(w).Encode(map[string]any{"matches": ids})
}
