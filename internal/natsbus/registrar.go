package natsbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// RegistrationSubjectPrefix is exported so the hub subscriber can
// match on it without hardcoding the literal.
const RegistrationSubjectPrefix = "trinity.register."

// RosterProvider lets the Registrar refresh its roster at publish time
// instead of locking it in at construction. A collector typically
// registers its Q3 servers in the hub's DB on startup, then produces
// this roster from the same source. Returning nil or an empty slice
// is fine — the hub will still see the heartbeat.
type RosterProvider func() []domain.RegdServer

// Registrar publishes a domain.Registration to trinity.register.<source>
// immediately on Start, then at the configured interval until Stop.
// One instance per collector process.
type Registrar struct {
	nc       *nats.Conn
	source   string
	uuid     string
	version  string
	roster   RosterProvider
	interval time.Duration

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewRegistrar constructs a Registrar.
func NewRegistrar(nc *nats.Conn, source, sourceUUID, version string, roster RosterProvider, interval time.Duration) (*Registrar, error) {
	if nc == nil {
		return nil, fmt.Errorf("natsbus.NewRegistrar: NATS connection is required")
	}
	if source == "" {
		return nil, fmt.Errorf("natsbus.NewRegistrar: source is required")
	}
	if sourceUUID == "" {
		return nil, fmt.Errorf("natsbus.NewRegistrar: source_uuid is required")
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if roster == nil {
		roster = func() []domain.RegdServer { return nil }
	}
	return &Registrar{
		nc:       nc,
		source:   source,
		uuid:     sourceUUID,
		version:  version,
		roster:   roster,
		interval: interval,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}, nil
}

// Start publishes once immediately, then on each interval tick until
// Stop is called or ctx cancels. Returns immediately.
func (r *Registrar) Start(ctx context.Context) {
	go r.run(ctx)
}

// Stop cancels the publish loop and waits for it to exit.
func (r *Registrar) Stop() {
	r.stopOnce.Do(func() { close(r.stopCh) })
	<-r.doneCh
}

func (r *Registrar) run(ctx context.Context) {
	defer close(r.doneCh)
	r.publish()
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-r.stopCh:
			return
		case <-ctx.Done():
			return
		case <-t.C:
			r.publish()
		}
	}
}

func (r *Registrar) publish() {
	reg := domain.Registration{
		Source:        r.source,
		SourceUUID:    r.uuid,
		Version:       r.version,
		SchemaVersion: domain.RegistrationSchemaVersion,
		Servers:       r.roster(),
	}
	body, err := json.Marshal(reg)
	if err != nil {
		log.Printf("natsbus.Registrar: marshal: %v", err)
		return
	}
	if err := r.nc.Publish(RegistrationSubjectPrefix+r.source, body); err != nil {
		log.Printf("natsbus.Registrar: publish: %v", err)
	}
}
