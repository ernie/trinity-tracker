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
// Phase 3 scope: Seq is an in-memory atomic counter seeded at
// construction. Persistence across restarts arrives in phase 7 with
// the publish watermark; until then, restarting the collector resets
// the counter and the dedup window may swallow early events after a
// fast restart. Acceptable for the transitional state; the phase plan
// notes it is not shippable on its own.
type Publisher struct {
	js         nats.JetStreamContext
	source     string
	sourceUUID string

	seqMu sync.Mutex
	seq   uint64
}

// NewPublisher constructs a Publisher. initialSeq is the value the
// next publish will *exceed* — i.e. the first emitted envelope will
// carry Seq=initialSeq+1. Phase 3 callers pass 0; phase 7 passes the
// persisted watermark.
func NewPublisher(nc *nats.Conn, source, sourceUUID string, initialSeq uint64) (*Publisher, error) {
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
	return nil
}

// LastSeq returns the most recently assigned sequence number. Useful
// for tests and for the phase 7 watermark persistence hook.
func (p *Publisher) LastSeq() uint64 {
	p.seqMu.Lock()
	defer p.seqMu.Unlock()
	return p.seq
}
