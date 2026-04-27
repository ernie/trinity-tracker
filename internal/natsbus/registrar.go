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

const RegistrationSubjectPrefix = "trinity.register."

// RosterProvider is called at each publish tick. Nil/empty is allowed.
type RosterProvider func() []domain.RegdServer

// Registrar publishes a domain.Registration to trinity.register.<source>
// on Start and then at interval. demoBaseURL is the operator-owned
// URL their demos serve from; changes propagate to the hub on the
// next heartbeat.
type Registrar struct {
	nc          *nats.Conn
	source      string
	version     string
	demoBaseURL string
	roster      RosterProvider
	interval    time.Duration

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

func NewRegistrar(nc *nats.Conn, source, version, demoBaseURL string, roster RosterProvider, interval time.Duration) (*Registrar, error) {
	if nc == nil {
		return nil, fmt.Errorf("natsbus.NewRegistrar: NATS connection is required")
	}
	if source == "" {
		return nil, fmt.Errorf("natsbus.NewRegistrar: source is required")
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if roster == nil {
		roster = func() []domain.RegdServer { return nil }
	}
	return &Registrar{
		nc:          nc,
		source:      source,
		version:     version,
		demoBaseURL: demoBaseURL,
		roster:      roster,
		interval:    interval,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}, nil
}

// Start fires an immediate publish, then ticks at interval.
func (r *Registrar) Start(ctx context.Context) {
	go r.run(ctx)
}

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
		Version:       r.version,
		SchemaVersion: domain.RegistrationSchemaVersion,
		DemoBaseURL:   r.demoBaseURL,
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
