package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/ernie/trinity-tracker/internal/auth"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/hub"
	"github.com/ernie/trinity-tracker/internal/natsbus"
)

// RconRequest is the RCON command request body.
type RconRequest struct {
	Command string `json:"command"`
}

// RconResponse carries a command's output.
type RconResponse struct {
	Output string `json:"output"`
}

// handleRconCommand runs an RCON command for the source's owner, or for
// an admin when the server allows hub admin RCON (the collector's
// allow_hub_admin_rcon, mirrored in servers.admin_delegation_enabled).
func (r *Router) handleRconCommand(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Log in first.")
		return
	}

	serverID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid server ID.")
		return
	}

	var rconReq RconRequest
	if err := json.NewDecoder(req.Body).Decode(&rconReq); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}
	if rconReq.Command == "" {
		writeError(w, http.StatusBadRequest, "Enter a command.")
		return
	}

	server, err := r.store.GetServerByID(req.Context(), serverID)
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

	if r.store != nil {
		uid := claims.UserID
		if logErr := r.store.WriteSourceAudit(req.Context(), server.Source, &uid, "rcon.exec", server.Key+": "+rconReq.Command); logErr != nil {
			log.Printf("rcon: source_audit insert failed: %v", logErr)
		}
	}

	writeJSON(w, http.StatusOK, RconResponse{Output: output})
}

// handleRconStatus reports whether the caller may use RCON on a server.
// Whether the server has an rconpassword is only known when a command
// is sent.
func (r *Router) handleRconStatus(w http.ResponseWriter, req *http.Request) {
	serverID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid server ID.")
		return
	}

	claims := r.getAuthClaims(req)
	if claims == nil {
		writeJSON(w, http.StatusOK, map[string]bool{"available": false})
		return
	}

	server, err := r.store.GetServerByID(req.Context(), serverID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]bool{"available": false})
		return
	}

	if _, err := r.authorizeRcon(req.Context(), server, claims); err != nil {
		writeJSON(w, http.StatusOK, map[string]bool{"available": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"available": true})
}

var errRconForbidden = errors.New("rcon: forbidden")

// AuthorizeUserForRcon returns the RCON role a user gets on a server,
// if any. It wraps hub.AuthorizeUserForRcon, which the handshake's
// rconpassword autoset also uses.
func AuthorizeUserForRcon(userID int64, isAdmin bool, server *domain.Server, localSource string, sourceOwners map[string]int64) (natsbus.RconRole, bool) {
	return hub.AuthorizeUserForRcon(userID, isAdmin, server, localSource, sourceOwners)
}

// authorizeRcon authorizes the request's user against a freshly read
// server row, since the operator can change admin delegation at any
// time.
func (r *Router) authorizeRcon(ctx context.Context, server *domain.Server, claims *auth.Claims) (natsbus.RconRole, error) {
	owners, err := r.store.SourceOwners(ctx)
	if err != nil {
		return "", err
	}
	role, ok := hub.AuthorizeUserForRcon(claims.UserID, claims.IsAdmin, server, r.localSource, owners)
	if !ok {
		return "", errRconForbidden
	}
	return role, nil
}
