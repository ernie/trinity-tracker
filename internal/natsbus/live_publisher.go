package natsbus

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// SubjectLivePrefix is the core-NATS subject base for ephemeral live
// events. Each source publishes to trinity.live.<source>. No
// JetStream, no durability: live events are dropped while NATS is
// disconnected and the operator sees them again on the next trigger.
const SubjectLivePrefix = "trinity.live."

// LivePublisher wraps domain.Event values in a domain.Envelope and
// publishes them as fire-and-forget messages. Implements
// hub.LiveEventPublisher.
type LivePublisher struct {
	nc         *nats.Conn
	source     string
	sourceUUID string
}

// NewLivePublisher binds a publisher to a source_id / source_uuid pair.
func NewLivePublisher(nc *nats.Conn, source, sourceUUID string) (*LivePublisher, error) {
	if nc == nil {
		return nil, fmt.Errorf("natsbus.NewLivePublisher: NATS connection is required")
	}
	if source == "" {
		return nil, fmt.Errorf("natsbus.NewLivePublisher: source is required")
	}
	if sourceUUID == "" {
		return nil, fmt.Errorf("natsbus.NewLivePublisher: source_uuid is required")
	}
	return &LivePublisher{nc: nc, source: source, sourceUUID: sourceUUID}, nil
}

// PublishLive serializes the event into an Envelope and fires it on
// trinity.live.<source>. Errors are returned to the caller, which
// decides whether to log and drop (typical) or handle differently.
func (p *LivePublisher) PublishLive(e domain.Event) error {
	payload, err := json.Marshal(e.Data)
	if err != nil {
		return fmt.Errorf("natsbus.LivePublisher: marshal %s payload: %w", e.Type, err)
	}
	env := domain.Envelope{
		SchemaVersion:  domain.EnvelopeSchemaVersion,
		Source:         p.source,
		SourceUUID:     p.sourceUUID,
		RemoteServerID: e.ServerID,
		Seq:            0, // live events are not deduplicated
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

// timestampForLive normalizes to UTC and falls back to the current
// wall-clock if the caller didn't set one (some emitEvent call sites
// populate Timestamp as the parsed log time, a few don't).
func timestampForLive(ts time.Time) time.Time {
	if ts.IsZero() {
		return time.Now().UTC()
	}
	return ts.UTC()
}
