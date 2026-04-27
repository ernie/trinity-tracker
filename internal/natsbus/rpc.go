package natsbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/hub"
)

// Default timeout for an RPC round-trip. NATS on local loopback is
// sub-ms; even transatlantic WAN hops stay well under a second. 2s
// gives plenty of margin while still failing fast if the hub is down.
const defaultRPCTimeout = 2 * time.Second

// RPCQueueGroup is the queue group all hub-side handlers join so only
// one hub instance services each request.
const RPCQueueGroup = "hub-rpc"

// RPC subjects. The source segment is the collector's source_id.
const (
	subjectGreetPrefix = "trinity.rpc.greet."
	subjectClaimPrefix = "trinity.rpc.claim."
	subjectLinkPrefix  = "trinity.rpc.link."

	subjectServerRegisterPrefix = "trinity.rpc.server.register."
	subjectIdentityUpsertPrefix = "trinity.rpc.identity.upsert."
	subjectIdentityUpsertBot    = "trinity.rpc.identity.upsert_bot."
	subjectIdentityLookupPrefix = "trinity.rpc.identity.lookup."
)

// RPCClient publishes req/reply messages on trinity.rpc.<kind>.<source>
// and marshals / unmarshals hub.* request-reply types. Implements
// hub.RPCClient and hub.ServerClient.
type RPCClient struct {
	nc         *nats.Conn
	source     string
	sourceUUID string
	timeout    time.Duration
}

// NewRPCClient binds a client to a specific source_id. timeout of 0
// uses the default (2s). sourceUUID is passed on RegisterServer so the
// hub can tag the row with (source_uuid, local_id) for envelope
// resolution; other methods don't need it (the subject carries source
// scope and the hub's data keys aren't source-scoped for these reads).
func NewRPCClient(nc *nats.Conn, source, sourceUUID string, timeout time.Duration) (*RPCClient, error) {
	if nc == nil {
		return nil, fmt.Errorf("natsbus.NewRPCClient: NATS connection is required")
	}
	if source == "" {
		return nil, fmt.Errorf("natsbus.NewRPCClient: source is required")
	}
	if timeout <= 0 {
		timeout = defaultRPCTimeout
	}
	return &RPCClient{nc: nc, source: source, sourceUUID: sourceUUID, timeout: timeout}, nil
}

// Greet satisfies hub.RPCClient.
func (c *RPCClient) Greet(ctx context.Context, req hub.GreetRequest) (hub.GreetReply, error) {
	var reply hub.GreetReply
	err := c.request(ctx, subjectGreetPrefix+c.source, req, &reply)
	return reply, err
}

// Claim satisfies hub.RPCClient.
func (c *RPCClient) Claim(ctx context.Context, req hub.ClaimRequest) (hub.ClaimReply, error) {
	var reply hub.ClaimReply
	err := c.request(ctx, subjectClaimPrefix+c.source, req, &reply)
	return reply, err
}

// Link satisfies hub.RPCClient.
func (c *RPCClient) Link(ctx context.Context, req hub.LinkRequest) (hub.LinkReply, error) {
	var reply hub.LinkReply
	err := c.request(ctx, subjectLinkPrefix+c.source, req, &reply)
	return reply, err
}

// RegisterServer satisfies hub.ServerClient.
func (c *RPCClient) RegisterServer(ctx context.Context, name, address, logPath string) (*domain.Server, error) {
	req := hub.RegisterServerRequest{
		SourceUUID: c.sourceUUID,
		Name:       name,
		Address:    address,
		LogPath:    logPath,
	}
	var reply hub.RegisterServerReply
	if err := c.request(ctx, subjectServerRegisterPrefix+c.source, req, &reply); err != nil {
		return nil, err
	}
	if reply.Error != "" {
		return nil, fmt.Errorf("hub.RegisterServer: %s", reply.Error)
	}
	return reply.Server, nil
}

// UpsertPlayerIdentity satisfies hub.ServerClient.
func (c *RPCClient) UpsertPlayerIdentity(ctx context.Context, guid, name, cleanName string, ts time.Time, isVR bool) (hub.PlayerIdentity, error) {
	req := hub.UpsertIdentityRequest{GUID: guid, Name: name, CleanName: cleanName, Timestamp: ts.UTC(), IsVR: isVR}
	var reply hub.IdentityReply
	if err := c.request(ctx, subjectIdentityUpsertPrefix+c.source, req, &reply); err != nil {
		return hub.PlayerIdentity{}, err
	}
	if reply.Error != "" {
		return hub.PlayerIdentity{}, fmt.Errorf("hub.UpsertPlayerIdentity: %s", reply.Error)
	}
	return reply.Identity, nil
}

// UpsertBotPlayerIdentity satisfies hub.ServerClient.
func (c *RPCClient) UpsertBotPlayerIdentity(ctx context.Context, name, cleanName string, ts time.Time) (hub.PlayerIdentity, error) {
	req := hub.UpsertBotIdentityRequest{Name: name, CleanName: cleanName, Timestamp: ts.UTC()}
	var reply hub.IdentityReply
	if err := c.request(ctx, subjectIdentityUpsertBot+c.source, req, &reply); err != nil {
		return hub.PlayerIdentity{}, err
	}
	if reply.Error != "" {
		return hub.PlayerIdentity{}, fmt.Errorf("hub.UpsertBotPlayerIdentity: %s", reply.Error)
	}
	return reply.Identity, nil
}

// LookupPlayerIdentity satisfies hub.ServerClient.
func (c *RPCClient) LookupPlayerIdentity(ctx context.Context, guid string) (hub.PlayerIdentity, error) {
	req := hub.LookupIdentityRequest{GUID: guid}
	var reply hub.IdentityReply
	if err := c.request(ctx, subjectIdentityLookupPrefix+c.source, req, &reply); err != nil {
		return hub.PlayerIdentity{}, err
	}
	if reply.Error != "" {
		return hub.PlayerIdentity{}, fmt.Errorf("hub.LookupPlayerIdentity: %s", reply.Error)
	}
	return reply.Identity, nil
}

func (c *RPCClient) request(ctx context.Context, subject string, req, reply interface{}) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("natsbus.RPCClient: marshal request: %w", err)
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	msg, err := c.nc.RequestWithContext(reqCtx, subject, body)
	if err != nil {
		return fmt.Errorf("natsbus.RPCClient: %s: %w", subject, err)
	}
	if err := json.Unmarshal(msg.Data, reply); err != nil {
		return fmt.Errorf("natsbus.RPCClient: unmarshal reply: %w", err)
	}
	return nil
}

// RPCHandlers is the hub-side contract a NATS handler invokes. *hub.Writer
// satisfies it naturally via its RPCClient and ServerClient method sets.
type RPCHandlers interface {
	hub.RPCClient
	hub.ServerClient
}

// RPCServer is the bundle of active NATS subscriptions. It is created
// by RegisterRPCHandlers and torn down by Stop.
type RPCServer struct {
	subs []*nats.Subscription
}

// RegisterRPCHandlers subscribes the hub's RPC handlers on
// trinity.rpc.{greet,claim,link,server,identity,session}.> in the
// shared hub-rpc queue group. Pass the returned *RPCServer to Stop on
// shutdown.
func RegisterRPCHandlers(nc *nats.Conn, h RPCHandlers) (*RPCServer, error) {
	if nc == nil {
		return nil, fmt.Errorf("natsbus.RegisterRPCHandlers: NATS connection is required")
	}
	if h == nil {
		return nil, fmt.Errorf("natsbus.RegisterRPCHandlers: handler is required")
	}
	s := &RPCServer{}

	subscribe := func(subject string, cb nats.MsgHandler) error {
		sub, err := nc.QueueSubscribe(subject, RPCQueueGroup, cb)
		if err != nil {
			return fmt.Errorf("natsbus: subscribe %s: %w", subject, err)
		}
		s.subs = append(s.subs, sub)
		return nil
	}

	if err := subscribe("trinity.rpc.greet.>", func(m *nats.Msg) {
		var req hub.GreetRequest
		if err := json.Unmarshal(m.Data, &req); err != nil {
			log.Printf("natsbus.RPC greet: bad request: %v", err)
			return
		}
		reply, err := h.Greet(context.Background(), req)
		if err != nil {
			log.Printf("natsbus.RPC greet: handler error: %v", err)
		}
		respond(m, reply)
	}); err != nil {
		s.Stop()
		return nil, err
	}

	if err := subscribe("trinity.rpc.claim.>", func(m *nats.Msg) {
		var req hub.ClaimRequest
		if err := json.Unmarshal(m.Data, &req); err != nil {
			log.Printf("natsbus.RPC claim: bad request: %v", err)
			return
		}
		reply, err := h.Claim(context.Background(), req)
		if err != nil {
			log.Printf("natsbus.RPC claim: handler error: %v", err)
		}
		respond(m, reply)
	}); err != nil {
		s.Stop()
		return nil, err
	}

	if err := subscribe("trinity.rpc.link.>", func(m *nats.Msg) {
		var req hub.LinkRequest
		if err := json.Unmarshal(m.Data, &req); err != nil {
			log.Printf("natsbus.RPC link: bad request: %v", err)
			return
		}
		reply, err := h.Link(context.Background(), req)
		if err != nil {
			log.Printf("natsbus.RPC link: handler error: %v", err)
		}
		respond(m, reply)
	}); err != nil {
		s.Stop()
		return nil, err
	}

	if err := subscribe("trinity.rpc.server.register.>", func(m *nats.Msg) {
		var req hub.RegisterServerRequest
		if err := json.Unmarshal(m.Data, &req); err != nil {
			log.Printf("natsbus.RPC server.register: bad request: %v", err)
			return
		}
		srv, err := h.RegisterServer(context.Background(), req.Name, req.Address, req.LogPath)
		reply := hub.RegisterServerReply{Server: srv}
		if err != nil {
			reply.Error = err.Error()
		} else if tagger, ok := h.(sourceTagger); ok && srv != nil && req.SourceUUID != "" {
			// Tag the row with (source_uuid, local_id = servers.id) so
			// envelopes published as RemoteServerID=servers.id resolve
			// on the hub side. Matches the standalone hub+collector
			// TagLocalServerSource loop in main.go.
			if err := tagger.TagLocalServerSource(context.Background(), srv.ID, req.SourceUUID, srv.ID); err != nil {
				log.Printf("natsbus.RPC server.register: tag source for server %d: %v", srv.ID, err)
			}
		}
		respond(m, reply)
	}); err != nil {
		s.Stop()
		return nil, err
	}

	if err := subscribe("trinity.rpc.identity.upsert.>", func(m *nats.Msg) {
		var req hub.UpsertIdentityRequest
		if err := json.Unmarshal(m.Data, &req); err != nil {
			log.Printf("natsbus.RPC identity.upsert: bad request: %v", err)
			return
		}
		id, err := h.UpsertPlayerIdentity(context.Background(), req.GUID, req.Name, req.CleanName, req.Timestamp, req.IsVR)
		reply := hub.IdentityReply{Identity: id}
		if err != nil {
			reply.Error = err.Error()
		}
		respond(m, reply)
	}); err != nil {
		s.Stop()
		return nil, err
	}

	if err := subscribe("trinity.rpc.identity.upsert_bot.>", func(m *nats.Msg) {
		var req hub.UpsertBotIdentityRequest
		if err := json.Unmarshal(m.Data, &req); err != nil {
			log.Printf("natsbus.RPC identity.upsert_bot: bad request: %v", err)
			return
		}
		id, err := h.UpsertBotPlayerIdentity(context.Background(), req.Name, req.CleanName, req.Timestamp)
		reply := hub.IdentityReply{Identity: id}
		if err != nil {
			reply.Error = err.Error()
		}
		respond(m, reply)
	}); err != nil {
		s.Stop()
		return nil, err
	}

	if err := subscribe("trinity.rpc.identity.lookup.>", func(m *nats.Msg) {
		var req hub.LookupIdentityRequest
		if err := json.Unmarshal(m.Data, &req); err != nil {
			log.Printf("natsbus.RPC identity.lookup: bad request: %v", err)
			return
		}
		id, err := h.LookupPlayerIdentity(context.Background(), req.GUID)
		reply := hub.IdentityReply{Identity: id}
		if err != nil {
			reply.Error = err.Error()
		}
		respond(m, reply)
	}); err != nil {
		s.Stop()
		return nil, err
	}

	// Flush so every QueueSubscribe is registered with the server
	// before RegisterRPCHandlers returns. Otherwise a client that
	// fires a request immediately after setup can hit
	// "no responders available" while the subscription is still
	// propagating.
	if err := nc.Flush(); err != nil {
		s.Stop()
		return nil, fmt.Errorf("natsbus: flush after subscribe: %w", err)
	}

	return s, nil
}

// sourceTagger is satisfied by any handler that exposes the
// (source_uuid, local_id) tag operation. The writer implements this by
// delegating to its store; other implementations can opt out by not
// exposing the method.
type sourceTagger interface {
	TagLocalServerSource(ctx context.Context, serverID int64, sourceUUID string, localID int64) error
}

// Stop tears down all RPC subscriptions. Safe to call more than once.
func (s *RPCServer) Stop() {
	if s == nil {
		return
	}
	for _, sub := range s.subs {
		_ = sub.Unsubscribe()
	}
	s.subs = nil
}

func respond(m *nats.Msg, reply interface{}) {
	body, err := json.Marshal(reply)
	if err != nil {
		log.Printf("natsbus.RPC: marshal reply: %v", err)
		return
	}
	if err := m.Respond(body); err != nil {
		log.Printf("natsbus.RPC: respond: %v", err)
	}
}
