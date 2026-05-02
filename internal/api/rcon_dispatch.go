package api

import (
	"context"
	"fmt"

	"github.com/ernie/trinity-tracker/internal/auth"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/natsbus"
)

// SetRconClient wires the hub-side NATS RCON proxy. Set in main.go
// when running with NATS; left nil in tests and in single-machine
// installs that have no remote sources to reach. When nil, dispatch
// falls back to the in-process manager (and errors for any source
// that isn't local).
func (r *Router) SetRconClient(c *natsbus.RconClient) {
	r.rconClient = c
}

// SetLocalSource marks one source name as in-process — RCON requests
// for it short-circuit through the local ServerManager rather than
// round-tripping through NATS. Empty (the default) means "no local
// source" and is correct for hub-only deployments.
func (r *Router) SetLocalSource(source string) {
	r.localSource = source
}

// dispatchRcon routes the proxy request: in-process for the local
// source, NATS for everything else. Caller is responsible for having
// already authorized the request.
func (r *Router) dispatchRcon(ctx context.Context, server *domain.Server, command string, claims *auth.Claims, role natsbus.RconRole) (string, error) {
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
