package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/ernie/trinity-tracker/internal/hub"
)

// ConsoleServer is one row of the console server list: a server the
// caller is authorized to rcon, with the role they'd act under.
type ConsoleServer struct {
	Source  string `json:"source"`
	Key     string `json:"key"`
	Address string `json:"address"`
	Active  bool   `json:"active"`
	Role    string `json:"role"`
}

// ConsoleRconRequest addresses a server the way the CLI does: by
// (source, key), not DB id.
type ConsoleRconRequest struct {
	Source  string `json:"source"`
	Key     string `json:"key"`
	Command string `json:"command"`
}

// handleConsoleServers lists the servers the caller may rcon. Servers
// the caller has no role on are omitted — the list is "what can I
// control", not "what exists" (that's GET /api/servers).
func (r *Router) handleConsoleServers(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	servers, err := r.store.GetServers(req.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	owners, err := r.store.SourceOwners(req.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := []ConsoleServer{}
	for i := range servers {
		srv := &servers[i]
		role, ok := hub.AuthorizeUserForRcon(claims.UserID, claims.IsAdmin, srv, r.localSource, owners)
		if !ok {
			continue
		}
		out = append(out, ConsoleServer{
			Source:  srv.Source,
			Key:     srv.Key,
			Address: srv.Address,
			Active:  srv.Active,
			Role:    string(role),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleConsoleRcon is the (source, key)-addressed twin of
// handleRconCommand: same authorization, same dispatch, same audit —
// only the server resolution differs.
func (r *Router) handleConsoleRcon(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var rconReq ConsoleRconRequest
	if err := json.NewDecoder(req.Body).Decode(&rconReq); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if rconReq.Source == "" || rconReq.Key == "" || rconReq.Command == "" {
		writeError(w, http.StatusBadRequest, "source, key, and command are required")
		return
	}

	server, err := r.store.GetServerBySourceKey(req.Context(), rconReq.Source, rconReq.Key)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}

	role, err := r.authorizeRcon(req.Context(), server, claims)
	if err != nil {
		if errors.Is(err, errRconForbidden) {
			writeError(w, http.StatusForbidden, "you do not have RCON access to this server")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	output, err := r.dispatchRcon(req.Context(), server, rconReq.Command, claims, role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	uid := claims.UserID
	if logErr := r.store.WriteSourceAudit(req.Context(), server.Source, &uid, "rcon.exec", server.Key+": "+rconReq.Command); logErr != nil {
		log.Printf("console rcon: source_audit insert failed: %v", logErr)
	}

	writeJSON(w, http.StatusOK, RconResponse{Output: output})
}
