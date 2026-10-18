package natsbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/ernie/trinity-tracker/internal/hub"
	"github.com/nats-io/nats.go"
)

// RCON requests run from hub to collector. The hub checks the caller's
// identity and decides who may ask; the collector holds the rcon
// password and rechecks hub-admin requests against its per-server
// allow_hub_admin_rcon flag, so a misconfigured hub can't grant rcon on
// its own.
//
// Requests go to trinity.rcon.exec.<source>; the collector may
// subscribe there and publish replies on _INBOX.>.
const (
	RconExecSubjectPrefix = "trinity.rcon.exec."
	defaultRconTimeout    = 5 * time.Second
)

// RconRole and its constants live in internal/hub, where
// AuthorizeUserForRcon serves both the HTTP rcon path and the
// handshake's rcon-autoset.
type RconRole = hub.RconRole

const (
	RconRoleOwner    = hub.RconRoleOwner
	RconRoleHubAdmin = hub.RconRoleHubAdmin
)

// RconExecRequest is the hub's RCON proxy request. ServerKey is the
// per-source stable key (matches Q3Server.Key in the collector's cfg).
type RconExecRequest struct {
	ServerKey string   `json:"server_key"`
	Command   string   `json:"command"`
	Username  string   `json:"username"`
	Role      RconRole `json:"role"`
}

// RconExecReply carries the rcon output or an Error. Both empty is a
// valid reply, since many rcon commands print nothing.
type RconExecReply struct {
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// RconClient sends RCON requests from the hub; Exec takes the source on
// each call.
type RconClient struct {
	nc      *nats.Conn
	timeout time.Duration
}

// NewRconClient builds a hub-side RCON proxy client. timeout <= 0 uses
// 5s, which allows for a busy game server.
func NewRconClient(nc *nats.Conn, timeout time.Duration) (*RconClient, error) {
	if nc == nil {
		return nil, fmt.Errorf("natsbus.NewRconClient: NATS connection is required")
	}
	if timeout <= 0 {
		timeout = defaultRconTimeout
	}
	return &RconClient{nc: nc, timeout: timeout}, nil
}

// Exec sends an RCON request to the source's collector and returns its
// output. A reply.Error becomes a Go error, so transport and command
// failures share one path.
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

// RconHandler is the collector side. Implementations look up the server
// by key, recheck the request against its config and run it on the game
// server.
type RconHandler interface {
	HandleRcon(ctx context.Context, req RconExecRequest) RconExecReply
}

// RconServer holds the collector's NATS subscription for RCON proxy
// requests.
type RconServer struct {
	sub *nats.Subscription
}

// RegisterRconHandler subscribes the collector to
// trinity.rcon.exec.<source>. Scoping by source keeps collectors on one
// NATS server from handling each other's requests.
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
