package collector

import (
	"context"
	"log"

	"github.com/ernie/trinity-tracker/internal/natsbus"
)

// RconProxyHandler answers hub-issued RCON proxy requests on
// trinity.rcon.exec.<source>. It re-validates the request against the
// collector's own per-server delegation flag — the hub already
// authenticated the user with JWT and decided role=owner|hub_admin,
// but the rcon password lives here, so the final say is also here.
//
// Owner role: trusted unconditionally. The hub knows the user owns
// the source (sources.owner_user_id == user.id); a non-owner asking
// to RCON gets rejected at the hub before we ever see it. We could
// re-verify by carrying the owner identity through, but that would
// duplicate the source-ownership SoT (which lives in the hub DB),
// not strengthen it.
//
// Hub admin role: validated against AllowHubAdminRcon for the named
// server. A flip to false in the operator's cfg propagates within one
// heartbeat to the hub UI; this layer is the safety net for the gap.
type RconProxyHandler struct {
	manager *ServerManager
}

// NewRconProxyHandler wires the handler to the manager. Caller passes
// the resulting handler to natsbus.RegisterRconHandler.
func NewRconProxyHandler(manager *ServerManager) *RconProxyHandler {
	return &RconProxyHandler{manager: manager}
}

// HandleRcon implements natsbus.RconHandler.
func (h *RconProxyHandler) HandleRcon(_ context.Context, req natsbus.RconExecRequest) natsbus.RconExecReply {
	if req.ServerKey == "" {
		return natsbus.RconExecReply{Error: "server_key is required"}
	}
	if req.Command == "" {
		return natsbus.RconExecReply{Error: "command is required"}
	}
	switch req.Role {
	case natsbus.RconRoleOwner:
		// Trust the hub: ownership was verified there.
	case natsbus.RconRoleHubAdmin:
		if !h.manager.AdminDelegationFor(req.ServerKey) {
			log.Printf("collector.rcon: refusing hub-admin RCON for %q (delegation disabled in cfg)", req.ServerKey)
			return natsbus.RconExecReply{Error: "admin delegation not enabled for this server"}
		}
	default:
		return natsbus.RconExecReply{Error: "unrecognized role"}
	}
	output, err := h.manager.ExecuteRconByKey(req.ServerKey, req.Command)
	if err != nil {
		return natsbus.RconExecReply{Error: err.Error()}
	}
	log.Printf("collector.rcon: %s %s ran %q on %s", req.Role, req.Username, req.Command, req.ServerKey)
	return natsbus.RconExecReply{Output: output}
}
