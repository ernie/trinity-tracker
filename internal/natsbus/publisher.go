package natsbus

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// Publisher serializes FactEvents into domain.Envelope values and
// publishes them to JetStream on trinity.events.<source> with a
// Nats-Msg-Id header of "<source>:<seq>" so JetStream's duplicate
// window can deduplicate retried publishes.
//
// A WatermarkTracker, when provided, records (seq, ts) after each
// successful publish so a crashed collector can resume from the next
// seq on restart and skip already-published log events during replay.
type Publisher struct {
	js         nats.JetStreamContext
	source     string
	sourceUUID string
	watermark  *WatermarkTracker

	seqMu sync.Mutex
	seq   uint64
}

// NewPublisher constructs a Publisher. initialSeq is the value the
// next publish will *exceed* — i.e. the first emitted envelope will
// carry Seq=initialSeq+1. Phase 7 callers pass watermark.LastSeq
// (via the tracker) so monotonicity survives restarts.
func NewPublisher(nc *nats.Conn, source, sourceUUID string, initialSeq uint64) (*Publisher, error) {
	return NewPublisherWithWatermark(nc, source, sourceUUID, initialSeq, nil)
}

// NewPublisherWithWatermark is the full constructor. Pass a non-nil
// tracker to enable watermark persistence on each successful publish.
func NewPublisherWithWatermark(nc *nats.Conn, source, sourceUUID string, initialSeq uint64, wm *WatermarkTracker) (*Publisher, error) {
	if nc == nil {
		return nil, fmt.Errorf("natsbus: NATS connection is required")
	}
	if source == "" {
		return nil, fmt.Errorf("natsbus: source is required")
	}
	if sourceUUID == "" {
		return nil, fmt.Errorf("natsbus: source_uuid is required")
	}
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("natsbus: JetStream context: %w", err)
	}
	return &Publisher{
		js:         js,
		source:     source,
		sourceUUID: sourceUUID,
		watermark:  wm,
		seq:        initialSeq,
	}, nil
}

// Publish serializes the event into an Envelope and publishes it to
// JetStream. The call blocks until the server acks or the underlying
// NATS client's publish timeout elapses. Errors are returned; the hub
// writer logs and drops.
func (p *Publisher) Publish(e domain.FactEvent) error {
	data, err := json.Marshal(e.Data)
	if err != nil {
		return fmt.Errorf("natsbus: marshal payload (%s): %w", e.Type, err)
	}

	p.seqMu.Lock()
	p.seq++
	seq := p.seq
	p.seqMu.Unlock()

	env := domain.Envelope{
		SchemaVersion:  domain.EnvelopeSchemaVersion,
		Source:         p.source,
		SourceUUID:     p.sourceUUID,
		RemoteServerID: e.ServerID,
		Seq:            seq,
		Timestamp:      e.Timestamp.UTC(),
		Event:          e.Type,
		Data:           data,
	}
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("natsbus: marshal envelope: %w", err)
	}

	msg := &nats.Msg{
		Subject: "trinity.events." + p.source,
		Data:    body,
		Header:  nats.Header{},
	}
	msg.Header.Set(nats.MsgIdHdr, p.source+":"+strconv.FormatUint(seq, 10))

	if _, err := p.js.PublishMsg(msg); err != nil {
		return fmt.Errorf("natsbus: publish %s seq=%d: %w", e.Type, seq, err)
	}
	if p.watermark != nil {
		if err := p.watermark.Update(seq, env.Timestamp); err != nil {
			return fmt.Errorf("natsbus: watermark update seq=%d: %w", seq, err)
		}
	}
	return nil
}

// LastSeq returns the most recently assigned sequence number. Useful
// for tests and for the phase 7 watermark persistence hook.
func (p *Publisher) LastSeq() uint64 {
	p.seqMu.Lock()
	defer p.seqMu.Unlock()
	return p.seq
}
