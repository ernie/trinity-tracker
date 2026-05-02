package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/ernie/trinity-tracker/internal/auth"
	"github.com/ernie/trinity-tracker/internal/domain"
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

// authorizeRcon decides whether claims can RCON server, returning the
// wire-level role to forward to the collector. The same rule lives in
// handleGetServers' manageable_by_me computation; keep them in sync
// so a clickable card never silently 403s.
//
// Reads admin_delegation_enabled directly off the freshly-fetched
// server row — the operator can flip the flag between the user
// landing on the page and clicking RCON, and the collector will
// re-validate again on the other side either way.
func (r *Router) authorizeRcon(ctx context.Context, server *domain.Server, claims *auth.Claims) (natsbus.RconRole, error) {
	owners, err := r.store.SourceOwners(ctx)
	if err != nil {
		return "", err
	}
	ownerID, hasOwner := owners[server.Source]
	isLocal := r.localSource != "" && server.Source == r.localSource
	switch {
	case hasOwner && ownerID == claims.UserID:
		return natsbus.RconRoleOwner, nil
	case claims.IsAdmin && isLocal:
		// Hub+collector: admin runs the local box, no opt-in required.
		return natsbus.RconRoleOwner, nil
	case claims.IsAdmin && server.AdminDelegationEnabled:
		// Remote source — owner-or-admin-minted alike — only when the
		// collector's cfg has opted in. Re-checked on the collector
		// side before the rcon password is touched.
		return natsbus.RconRoleHubAdmin, nil
	default:
		return "", errRconForbidden
	}
}
