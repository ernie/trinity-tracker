package natsbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/domain"
)

const (
	ConsumerName   = "hub-writer"
	maxAckPending  = 256
	fetchBatchSize = 32
	fetchWait      = 5 * time.Second
	ackWait        = 30 * time.Second
)

// EnvelopeHandler processes one decoded envelope. A nil return acks;
// non-nil Naks (redelivered after ackWait).
type EnvelopeHandler interface {
	HandleEnvelope(ctx context.Context, env domain.Envelope) error
}

// Subscriber is the hub-side durable pull consumer for TRINITY_EVENTS.
type Subscriber struct {
	js      nats.JetStreamContext
	handler EnvelopeHandler
	sub     *nats.Subscription

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

func NewSubscriber(nc *nats.Conn, handler EnvelopeHandler) (*Subscriber, error) {
	if nc == nil {
		return nil, fmt.Errorf("natsbus.NewSubscriber: NATS connection is required")
	}
	if handler == nil {
		return nil, fmt.Errorf("natsbus.NewSubscriber: handler is required")
	}
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("natsbus.NewSubscriber: JetStream context: %w", err)
	}
	sub, err := js.PullSubscribe(
		"trinity.events.>",
		ConsumerName,
		nats.BindStream(StreamEvents),
		nats.ManualAck(),
		nats.MaxAckPending(maxAckPending),
		nats.AckWait(ackWait),
	)
	if err != nil {
		return nil, fmt.Errorf("natsbus.NewSubscriber: PullSubscribe: %w", err)
	}
	return &Subscriber{
		js:      js,
		handler: handler,
		sub:     sub,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}, nil
}

func (s *Subscriber) Start(ctx context.Context) {
	go s.run(ctx)
}

// Stop cancels the fetch loop, unsubscribes, and waits for exit.
func (s *Subscriber) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	if s.sub != nil {
		_ = s.sub.Unsubscribe()
	}
	<-s.doneCh
}

func (s *Subscriber) run(ctx context.Context) {
	defer close(s.doneCh)
	for {
		select {
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		msgs, err := s.sub.Fetch(fetchBatchSize, nats.MaxWait(fetchWait))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) {
				continue
			}
			select {
			case <-s.stopCh:
				return
			case <-ctx.Done():
				return
			default:
			}
			log.Printf("natsbus.Subscriber: fetch: %v", err)
			continue
		}
		for _, m := range msgs {
			s.processMessage(ctx, m)
		}
	}
}

func (s *Subscriber) processMessage(ctx context.Context, m *nats.Msg) {
	var env domain.Envelope
	if err := json.Unmarshal(m.Data, &env); err != nil {
		log.Printf("natsbus.Subscriber: discarding unparseable envelope: %v", err)
		_ = m.Ack()
		return
	}
	if err := s.handler.HandleEnvelope(ctx, env); err != nil {
		log.Printf("natsbus.Subscriber: handle %s seq=%d source=%s: %v", env.Event, env.Seq, env.Source, err)
		_ = m.Nak()
		return
	}
	_ = m.Ack()
}
