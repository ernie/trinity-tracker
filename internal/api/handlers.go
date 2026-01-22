package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/ernie/trinity-tools/internal/storage"
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

// handleGetServers returns all servers
func (r *Router) handleGetServers(w http.ResponseWriter, req *http.Request) {
	servers, err := r.store.GetServers(req.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, servers)
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

	status := r.manager.GetServerStatus(id)
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

	status := r.manager.GetServerStatus(id)
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

	matches, err := r.store.GetFilteredMatchSummaries(req.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
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
	writeJSON(w, http.StatusOK, match)
}

// handleGetLeaderboard returns top players by specified category and time period
func (r *Router) handleGetLeaderboard(w http.ResponseWriter, req *http.Request) {
	limit := parseLimit(req, 50, 100)

	category := req.URL.Query().Get("category")
	if category == "" {
		category = "kills"
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

	botsOnly := req.URL.Query().Get("bots_only") == "true"

	response, err := r.store.GetLeaderboard(req.Context(), category, period, limit, botsOnly, gameType)
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

	writeJSON(w, http.StatusOK, matches)
}
