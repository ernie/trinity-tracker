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

// RconRequest is the request body for RCON commands
type RconRequest struct {
	Command string `json:"command"`
}

// RconResponse is the response body for RCON commands
type RconResponse struct {
	Output string `json:"output"`
}

// handleRconCommand executes an RCON command. The route is requireAuth
// (not requireAdmin) because non-admin users can RCON servers on
// sources they own. Admin access to a remote source's server is
// gated by the collector's per-server allow_hub_admin_rcon flag,
// which is mirrored on the hub via servers.admin_delegation_enabled.
func (r *Router) handleRconCommand(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

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

	server, err := r.store.GetServerByID(req.Context(), serverID)
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

	if r.store != nil {
		uid := claims.UserID
		if logErr := r.store.WriteSourceAudit(req.Context(), server.Source, &uid, "rcon.exec", server.Key+": "+rconReq.Command); logErr != nil {
			log.Printf("rcon: source_audit insert failed: %v", logErr)
		}
	}

	writeJSON(w, http.StatusOK, RconResponse{Output: output})
}

// handleRconStatus returns whether RCON is available for a server.
// Returns available=true when the caller is authorized (owner or
// admin-with-delegation); the actual rcon-password check happens at
// dispatch time on whichever box holds the password.
func (r *Router) handleRconStatus(w http.ResponseWriter, req *http.Request) {
	serverID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid server id")
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

// AuthorizeUserForRcon is the policy decision: given a user (by id +
// admin bit) and a server, what RCON role (if any) should the collector
// honor? Re-exported from internal/hub so existing api callers (and the
// tests in rcon_authorize_test.go) keep working at the api.* path; the
// implementation lives one layer down so the handshake-time autoset
// path can call it without an import cycle.
func AuthorizeUserForRcon(userID int64, isAdmin bool, server *domain.Server, localSource string, sourceOwners map[string]int64) (natsbus.RconRole, bool) {
	return hub.AuthorizeUserForRcon(userID, isAdmin, server, localSource, sourceOwners)
}

// authorizeRcon is the JWT-flavored wrapper around AuthorizeUserForRcon.
// Reads admin_delegation_enabled directly off the freshly-fetched server
// row — the operator can flip the flag between the user landing on the
// page and clicking RCON.
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
