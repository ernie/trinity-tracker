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

const defaultRPCTimeout = 2 * time.Second

// RPCQueueGroup lets multiple hub instances share a single consumer.
const RPCQueueGroup = "hub-rpc"

const (
	subjectGreetPrefix          = "trinity.rpc.greet."
	subjectClaimPrefix          = "trinity.rpc.claim."
	subjectLinkPrefix           = "trinity.rpc.link."
	subjectServerRegisterPrefix = "trinity.rpc.server.register."
	subjectIdentityUpsertPrefix = "trinity.rpc.identity.upsert."
	subjectIdentityUpsertBot    = "trinity.rpc.identity.upsert_bot."
	subjectIdentityLookupPrefix = "trinity.rpc.identity.lookup."
	subjectSourceProgressPrefix = "trinity.rpc.source.progress."
)

// RPCClient issues NATS req/reply on trinity.rpc.<kind>.<source>.
// Satisfies hub.RPCClient and hub.ServerClient.
type RPCClient struct {
	nc      *nats.Conn
	source  string
	timeout time.Duration
}

// NewRPCClient binds a client to a source. timeout <= 0 uses 2s.
func NewRPCClient(nc *nats.Conn, source string, timeout time.Duration) (*RPCClient, error) {
	if nc == nil {
		return nil, fmt.Errorf("natsbus.NewRPCClient: NATS connection is required")
	}
	if source == "" {
		return nil, fmt.Errorf("natsbus.NewRPCClient: source is required")
	}
	if timeout <= 0 {
		timeout = defaultRPCTimeout
	}
	return &RPCClient{nc: nc, source: source, timeout: timeout}, nil
}

func (c *RPCClient) Greet(ctx context.Context, req hub.GreetRequest) (hub.GreetReply, error) {
	var reply hub.GreetReply
	err := c.request(ctx, subjectGreetPrefix+c.source, req, &reply)
	return reply, err
}

func (c *RPCClient) Claim(ctx context.Context, req hub.ClaimRequest) (hub.ClaimReply, error) {
	var reply hub.ClaimReply
	err := c.request(ctx, subjectClaimPrefix+c.source, req, &reply)
	return reply, err
}

func (c *RPCClient) Link(ctx context.Context, req hub.LinkRequest) (hub.LinkReply, error) {
	var reply hub.LinkReply
	err := c.request(ctx, subjectLinkPrefix+c.source, req, &reply)
	return reply, err
}

func (c *RPCClient) RegisterServer(ctx context.Context, source, key, address string) (*domain.Server, error) {
	if source == "" {
		source = c.source
	}
	req := hub.RegisterServerRequest{
		Source:  source,
		Key:     key,
		Address: address,
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

func (c *RPCClient) GetSourceProgress(ctx context.Context, source string) (hub.SourceProgressReply, error) {
	if source == "" {
		source = c.source
	}
	req := hub.SourceProgressRequest{Source: source}
	var reply hub.SourceProgressReply
	if err := c.request(ctx, subjectSourceProgressPrefix+c.source, req, &reply); err != nil {
		return hub.SourceProgressReply{}, err
	}
	if reply.Error != "" {
		return hub.SourceProgressReply{}, fmt.Errorf("hub.GetSourceProgress: %s", reply.Error)
	}
	return reply, nil
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

// RPCHandlers is the hub-side contract invoked by NATS handlers.
type RPCHandlers interface {
	hub.RPCClient
	hub.ServerClient
}

type RPCServer struct {
	subs []*nats.Subscription
}

// RegisterRPCHandlers subscribes the hub-rpc queue group on every
// trinity.rpc.* subject the collector uses.
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
		srv, err := h.RegisterServer(context.Background(), req.Source, req.Key, req.Address)
		reply := hub.RegisterServerReply{Server: srv}
		if err != nil {
			reply.Error = err.Error()
		} else if tagger, ok := h.(sourceTagger); ok && srv != nil && req.Source != "" {
			if err := tagger.TagLocalServerSource(context.Background(), srv.ID, req.Source, srv.ID); err != nil {
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

	if err := subscribe("trinity.rpc.source.progress.>", func(m *nats.Msg) {
		var req hub.SourceProgressRequest
		if err := json.Unmarshal(m.Data, &req); err != nil {
			log.Printf("natsbus.RPC source.progress: bad request: %v", err)
			return
		}
		reply, err := h.GetSourceProgress(context.Background(), req.Source)
		if err != nil {
			reply.Error = err.Error()
		}
		respond(m, reply)
	}); err != nil {
		s.Stop()
		return nil, err
	}

	// Flush so all subscriptions are live before we return; otherwise
	// an immediate client request can hit "no responders available".
	if err := nc.Flush(); err != nil {
		s.Stop()
		return nil, fmt.Errorf("natsbus: flush after subscribe: %w", err)
	}

	return s, nil
}

type sourceTagger interface {
	TagLocalServerSource(ctx context.Context, serverID int64, source string, localID int64) error
}

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
