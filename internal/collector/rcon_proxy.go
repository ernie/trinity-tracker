package collector

import (
	"context"
	"fmt"
	"log"

	"github.com/ernie/trinity-tracker/internal/natsbus"
)

// RconProxyHandler answers hub-issued RCON requests on
// trinity.rcon.exec.<source>. The hub authenticates the user and picks
// the role, but the rcon password lives here, so the collector makes
// the final call.
//
// Owners are trusted: the hub checks source ownership against its own
// database before a request reaches the collector.
//
// Hub admins are checked against AllowHubAdminRcon for the server. A
// change in the operator's config reaches the hub UI on the next
// heartbeat; this check covers the time in between.
type RconProxyHandler struct {
	manager *ServerManager
}

// NewRconProxyHandler returns a handler for natsbus.RegisterRconHandler.
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
		// Ownership was verified at the hub.
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

// AuthorizeConsole implements natsbus.ConsoleAuthorizer. Console access
// follows the rcon delegation flag, since anyone who can rcon a server
// can already see its console output.
func (h *RconProxyHandler) AuthorizeConsole(serverKey string, role natsbus.RconRole) error {
	switch role {
	case natsbus.RconRoleOwner:
		return nil
	case natsbus.RconRoleHubAdmin:
		if !h.manager.AdminDelegationFor(serverKey) {
			return fmt.Errorf("admin delegation not enabled for this server")
		}
		return nil
	default:
		return fmt.Errorf("unrecognized role")
	}
}
