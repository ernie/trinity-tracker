package api

import (
	"context"
	"fmt"

	"github.com/ernie/trinity-tracker/internal/auth"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/natsbus"
)

// SetRconClient plugs in the NATS RCON proxy. Without one, only the
// local source can take RCON commands.
func (r *Router) SetRconClient(c *natsbus.RconClient) {
	r.rconClient = c
}

// SetLocalSource names the source this process collects for; its RCON
// goes straight to the local ServerManager. A hub with no collector
// leaves it empty.
func (r *Router) SetLocalSource(source string) {
	r.localSource = source
}

// dispatchRcon sends an already authorized command to the local
// ServerManager or, for other sources, over NATS.
func (r *Router) dispatchRcon(ctx context.Context, server *domain.Server, command string, claims *auth.Claims, role natsbus.RconRole) (string, error) {
	if r.testRconDispatch != nil {
		return r.testRconDispatch(server, command, role)
	}
	if r.localSource != "" && server.Source == r.localSource && r.manager != nil {
		return r.manager.ExecuteRconByKey(server.Key, command)
	}
	if r.rconClient == nil {
		return "", fmt.Errorf("rcon: no transport configured for source %q", server.Source)
	}
	return r.rconClient.Exec(ctx, server.Source, natsbus.RconExecRequest{
		ServerKey: server.Key,
		Command:   command,
		Username:  claims.Username,
		Role:      role,
	})
}
