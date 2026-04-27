package natsbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/domain"
)

type RegistrationHandler interface {
	HandleRegistration(ctx context.Context, reg domain.Registration) error
}

// RegistrationSubscriber listens on trinity.register.> (core NATS, not
// JetStream — late hubs resync within one heartbeat interval).
type RegistrationSubscriber struct {
	sub *nats.Subscription
}

func NewRegistrationSubscriber(nc *nats.Conn, h RegistrationHandler) (*RegistrationSubscriber, error) {
	if nc == nil {
		return nil, fmt.Errorf("natsbus.NewRegistrationSubscriber: NATS connection is required")
	}
	if h == nil {
		return nil, fmt.Errorf("natsbus.NewRegistrationSubscriber: handler is required")
	}
	sub, err := nc.Subscribe("trinity.register.>", func(m *nats.Msg) {
		var reg domain.Registration
		if err := json.Unmarshal(m.Data, &reg); err != nil {
			log.Printf("natsbus.Registration: bad payload on %s: %v", m.Subject, err)
			return
		}
		if err := h.HandleRegistration(context.Background(), reg); err != nil {
			log.Printf("natsbus.Registration: handle %s (source=%s): %v", m.Subject, reg.Source, err)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("natsbus: subscribe register: %w", err)
	}
	return &RegistrationSubscriber{sub: sub}, nil
}

func (s *RegistrationSubscriber) Stop() {
	if s == nil || s.sub == nil {
		return
	}
	_ = s.sub.Unsubscribe()
	s.sub = nil
}
