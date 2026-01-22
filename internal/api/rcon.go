package api

import (
	"encoding/json"
	"net/http"
)

// RconRequest is the request body for RCON commands
type RconRequest struct {
	Command string `json:"command"`
}

// RconResponse is the response body for RCON commands
type RconResponse struct {
	Output string `json:"output"`
}

// handleRconCommand executes an RCON command on a server (auth required)
func (r *Router) handleRconCommand(w http.ResponseWriter, req *http.Request) {
	serverID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid server id")
		return
	}

	var rconReq RconRequest
	if err := json.NewDecoder(req.Body).Decode(&rconReq); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if rconReq.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	output, err := r.manager.ExecuteRcon(serverID, rconReq.Command)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, RconResponse{Output: output})
}

// handleRconStatus returns whether RCON is available for a server (no auth needed)
func (r *Router) handleRconStatus(w http.ResponseWriter, req *http.Request) {
	serverID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid server id")
		return
	}

	hasRcon := r.manager.HasRconAccess(serverID)
	writeJSON(w, http.StatusOK, map[string]bool{"available": hasRcon})
}
