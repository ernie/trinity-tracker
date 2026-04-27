package natsbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"

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
)

// RPCClient publishes req/reply messages on trinity.rpc.<kind>.<source>
// and marshals / unmarshals hub.* request-reply types. Implements
// hub.RPCClient.
type RPCClient struct {
	nc      *nats.Conn
	source  string
	timeout time.Duration
}

// NewRPCClient binds a client to a specific source_id. timeout of 0
// uses the default (2s).
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
// satisfies it naturally.
type RPCHandlers interface {
	Greet(ctx context.Context, req hub.GreetRequest) (hub.GreetReply, error)
	Claim(ctx context.Context, req hub.ClaimRequest) (hub.ClaimReply, error)
	Link(ctx context.Context, req hub.LinkRequest) (hub.LinkReply, error)
}

// RPCServer is the bundle of active NATS subscriptions. It is created
// by RegisterRPCHandlers and torn down by Stop.
type RPCServer struct {
	subs []*nats.Subscription
}

// RegisterRPCHandlers subscribes the hub's RPC handlers on
// trinity.rpc.{greet,claim,link}.> in the shared hub-rpc queue group.
// Pass the returned *RPCServer to Stop on shutdown.
func RegisterRPCHandlers(nc *nats.Conn, h RPCHandlers) (*RPCServer, error) {
	if nc == nil {
		return nil, fmt.Errorf("natsbus.RegisterRPCHandlers: NATS connection is required")
	}
	if h == nil {
		return nil, fmt.Errorf("natsbus.RegisterRPCHandlers: handler is required")
	}
	s := &RPCServer{}

	greetSub, err := nc.QueueSubscribe("trinity.rpc.greet.>", RPCQueueGroup, func(m *nats.Msg) {
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
	})
	if err != nil {
		return nil, fmt.Errorf("natsbus: subscribe greet: %w", err)
	}
	s.subs = append(s.subs, greetSub)

	claimSub, err := nc.QueueSubscribe("trinity.rpc.claim.>", RPCQueueGroup, func(m *nats.Msg) {
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
	})
	if err != nil {
		s.Stop()
		return nil, fmt.Errorf("natsbus: subscribe claim: %w", err)
	}
	s.subs = append(s.subs, claimSub)

	linkSub, err := nc.QueueSubscribe("trinity.rpc.link.>", RPCQueueGroup, func(m *nats.Msg) {
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
	})
	if err != nil {
		s.Stop()
		return nil, fmt.Errorf("natsbus: subscribe link: %w", err)
	}
	s.subs = append(s.subs, linkSub)

	return s, nil
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
