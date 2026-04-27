package natsbus

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// Publisher serializes FactEvents into Envelopes and publishes them to
// trinity.events.<source> with Nats-Msg-Id "<source>:<seq>" for
// duplicate-window dedup. When a WatermarkTracker is set, (seq, ts)
// are persisted after each successful publish.
type Publisher struct {
	js        nats.JetStreamContext
	source    string
	watermark *WatermarkTracker

	seqMu sync.Mutex
	seq   uint64
}

// NewPublisher constructs a Publisher. The first emitted envelope
// carries Seq=initialSeq+1; pass watermark.LastSeq to survive restarts.
func NewPublisher(nc *nats.Conn, source string, initialSeq uint64) (*Publisher, error) {
	return NewPublisherWithWatermark(nc, source, initialSeq, nil)
}

func NewPublisherWithWatermark(nc *nats.Conn, source string, initialSeq uint64, wm *WatermarkTracker) (*Publisher, error) {
	if nc == nil {
		return nil, fmt.Errorf("natsbus: NATS connection is required")
	}
	if source == "" {
		return nil, fmt.Errorf("natsbus: source is required")
	}
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("natsbus: JetStream context: %w", err)
	}
	return &Publisher{
		js:        js,
		source:    source,
		watermark: wm,
		seq:       initialSeq,
	}, nil
}

// Publish serializes the event and publishes to JetStream, blocking
// until ack or publish timeout.
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

// LastSeq returns the most recently assigned sequence number.
func (p *Publisher) LastSeq() uint64 {
	p.seqMu.Lock()
	defer p.seqMu.Unlock()
	return p.seq
}
