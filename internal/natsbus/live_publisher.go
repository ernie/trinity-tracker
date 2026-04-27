package natsbus

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// SubjectLivePrefix is the core-NATS (non-JetStream) base subject.
// Live events are fire-and-forget; disconnected publishers drop them.
const SubjectLivePrefix = "trinity.live."

// LivePublisher wraps events in an Envelope and publishes them to
// trinity.live.<source>. Implements hub.LiveEventPublisher.
type LivePublisher struct {
	nc     *nats.Conn
	source string
}

func NewLivePublisher(nc *nats.Conn, source string) (*LivePublisher, error) {
	if nc == nil {
		return nil, fmt.Errorf("natsbus.NewLivePublisher: NATS connection is required")
	}
	if source == "" {
		return nil, fmt.Errorf("natsbus.NewLivePublisher: source is required")
	}
	return &LivePublisher{nc: nc, source: source}, nil
}

func (p *LivePublisher) PublishLive(e domain.Event) error {
	payload, err := json.Marshal(e.Data)
	if err != nil {
		return fmt.Errorf("natsbus.LivePublisher: marshal %s payload: %w", e.Type, err)
	}
	env := domain.Envelope{
		SchemaVersion:  domain.EnvelopeSchemaVersion,
		Source:         p.source,
		RemoteServerID: e.ServerID,
		Seq:            0,
		Timestamp:      timestampForLive(e.Timestamp),
		Event:          e.Type,
		Data:           payload,
	}
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("natsbus.LivePublisher: marshal envelope: %w", err)
	}
	return p.nc.Publish(SubjectLivePrefix+p.source, body)
}

// timestampForLive normalizes to UTC, substituting now if unset.
func timestampForLive(ts time.Time) time.Time {
	if ts.IsZero() {
		return time.Now().UTC()
	}
	return ts.UTC()
}
