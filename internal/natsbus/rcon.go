package natsbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

// RCON request-reply runs hub → collector. The hub holds the JWT-
// validated identity and decides who is allowed to ask; the collector
// holds the rcon password and re-validates the request against the
// per-server allow_hub_admin_rcon flag in its own config. Two checks
// instead of one is intentional: a misconfigured hub can't grant
// itself rcon, and a compromised collector still can't dispatch rcon
// without a corresponding hub-side authenticated request.
//
// Subject layout: trinity.rcon.exec.<source>. The collector's
// permissions allow Sub on this subject and Pub on _INBOX.> for the
// reply (standard NATS request-reply pattern with random reply tokens).
const (
	RconExecSubjectPrefix = "trinity.rcon.exec."
	defaultRconTimeout    = 5 * time.Second
)

// RconRole is what the hub asserts about the calling user. The
// collector trusts the hub on identity (JWT was already validated)
// but re-checks RoleHubAdmin against its per-server delegation flag.
type RconRole string

const (
	RconRoleOwner    RconRole = "owner"
	RconRoleHubAdmin RconRole = "hub_admin"
)

// RconExecRequest is the hub's RCON proxy request. ServerKey is the
// per-source stable key (matches Q3Server.Key in the collector's cfg).
type RconExecRequest struct {
	ServerKey string   `json:"server_key"`
	Command   string   `json:"command"`
	Username  string   `json:"username"`
	Role      RconRole `json:"role"`
}

// RconExecReply carries the rcon output OR a non-empty Error string.
// Empty Error + empty Output is a valid q3 response (rcon commands
// often produce no output).
type RconExecReply struct {
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// RconClient is the hub-side request issuer. Bind once per source the
// hub may target, or pass source to Exec each call — the latter avoids
// a client-per-source map at the call site.
type RconClient struct {
	nc      *nats.Conn
	timeout time.Duration
}

// NewRconClient builds a hub-side RCON proxy client. timeout <= 0
// uses the package default (5s — generous because q3 servers are
// occasionally slow under load).
func NewRconClient(nc *nats.Conn, timeout time.Duration) (*RconClient, error) {
	if nc == nil {
		return nil, fmt.Errorf("natsbus.NewRconClient: NATS connection is required")
	}
	if timeout <= 0 {
		timeout = defaultRconTimeout
	}
	return &RconClient{nc: nc, timeout: timeout}, nil
}

// Exec dispatches an RCON request to the given source's collector and
// returns its output. A non-empty reply.Error becomes a Go error so
// callers can write the same code path for transport and command-side
// failures.
func (c *RconClient) Exec(ctx context.Context, source string, req RconExecRequest) (string, error) {
	if source == "" {
		return "", fmt.Errorf("natsbus.RconClient.Exec: source is required")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("natsbus.RconClient.Exec: marshal: %w", err)
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	msg, err := c.nc.RequestWithContext(reqCtx, RconExecSubjectPrefix+source, body)
	if err != nil {
		return "", fmt.Errorf("natsbus.RconClient.Exec: %s: %w", source, err)
	}
	var reply RconExecReply
	if err := json.Unmarshal(msg.Data, &reply); err != nil {
		return "", fmt.Errorf("natsbus.RconClient.Exec: unmarshal reply: %w", err)
	}
	if reply.Error != "" {
		return reply.Output, fmt.Errorf("rcon: %s", reply.Error)
	}
	return reply.Output, nil
}

// RconHandler is the collector-side contract. Implementations look up
// the server by key, re-validate the request against per-server cfg,
// and execute via their q3 client.
type RconHandler interface {
	HandleRcon(ctx context.Context, req RconExecRequest) RconExecReply
}

// RconServer holds the collector's NATS subscription for RCON proxy
// requests.
type RconServer struct {
	sub *nats.Subscription
}

// RegisterRconHandler subscribes the collector to its RCON proxy
// subject (trinity.rcon.exec.<source>). One handler per collector
// process — the source argument scopes the subscription so two
// collectors on the same NATS won't double-handle each other's
// requests.
func RegisterRconHandler(nc *nats.Conn, source string, h RconHandler) (*RconServer, error) {
	if nc == nil {
		return nil, fmt.Errorf("natsbus.RegisterRconHandler: NATS connection is required")
	}
	if source == "" {
		return nil, fmt.Errorf("natsbus.RegisterRconHandler: source is required")
	}
	if h == nil {
		return nil, fmt.Errorf("natsbus.RegisterRconHandler: handler is required")
	}
	sub, err := nc.Subscribe(RconExecSubjectPrefix+source, func(m *nats.Msg) {
		var req RconExecRequest
		if err := json.Unmarshal(m.Data, &req); err != nil {
			respond(m, RconExecReply{Error: "invalid request"})
			return
		}
		reply := h.HandleRcon(context.Background(), req)
		respond(m, reply)
	})
	if err != nil {
		return nil, fmt.Errorf("natsbus: subscribe rcon: %w", err)
	}
	if err := nc.Flush(); err != nil {
		_ = sub.Unsubscribe()
		return nil, fmt.Errorf("natsbus: flush rcon subscription: %w", err)
	}
	log.Printf("natsbus: collector subscribed to %s%s", RconExecSubjectPrefix, source)
	return &RconServer{sub: sub}, nil
}

func (s *RconServer) Stop() {
	if s == nil || s.sub == nil {
		return
	}
	_ = s.sub.Unsubscribe()
	s.sub = nil
}
