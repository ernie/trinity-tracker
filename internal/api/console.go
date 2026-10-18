package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/ernie/trinity-tracker/internal/hub"
)

// ConsoleServer is a server the caller may use RCON on, with the role
// they act under.
type ConsoleServer struct {
	Source  string `json:"source"`
	Key     string `json:"key"`
	Address string `json:"address"`
	Active  bool   `json:"active"`
	Role    string `json:"role"`
}

// ConsoleRconRequest addresses a server by (source, key), as the CLI
// does.
type ConsoleRconRequest struct {
	Source  string `json:"source"`
	Key     string `json:"key"`
	Command string `json:"command"`
}

// handleConsoleServers lists only the servers the caller may use RCON on.
func (r *Router) handleConsoleServers(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Log in first.")
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

// handleConsoleRcon is handleRconCommand with the server addressed by
// (source, key).
func (r *Router) handleConsoleRcon(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Log in first.")
		return
	}

	var rconReq ConsoleRconRequest
	if err := json.NewDecoder(req.Body).Decode(&rconReq); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}
	if rconReq.Source == "" || rconReq.Key == "" || rconReq.Command == "" {
		writeError(w, http.StatusBadRequest, "Source, key and command are required.")
		return
	}

	server, err := r.store.GetServerBySourceKey(req.Context(), rconReq.Source, rconReq.Key)
	if err != nil {
		writeError(w, http.StatusNotFound, "Server not found.")
		return
	}

	role, err := r.authorizeRcon(req.Context(), server, claims)
	if err != nil {
		if errors.Is(err, errRconForbidden) {
			writeError(w, http.StatusForbidden, "You don't have RCON access to this server.")
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
