package hub

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// HandleRegistration processes a collector heartbeat. A source with no
// approved row is refused and logged, as a backstop to NATS auth.
// Known sources touch their heartbeat and upsert their server roster.
func (w *Writer) HandleRegistration(ctx context.Context, reg domain.Registration) error {
	if reg.Source == "" {
		return fmt.Errorf("hub.registration: missing source")
	}
	approved, err := w.store.IsSourceApproved(ctx, reg.Source)
	if err != nil {
		return err
	}
	if !approved {
		log.Printf("hub: registration from unprovisioned source=%q refused; an admin must create the source first", reg.Source)
		return nil
	}
	if err := w.store.TouchSourceHeartbeat(ctx, reg.Source, time.Now().UTC(), reg.Version, reg.DemoBaseURL); err != nil {
		return err
	}
	if w.liveState != nil {
		for _, srv := range reg.Servers {
			w.liveState.Set(reg.Source, srv.Key, srv.IsLive)
			w.liveState.SetDelay(reg.Source, srv.Key, srv.LiveDelaySeconds)
			w.liveState.SetViewers(reg.Source, srv.Key, srv.LiveViewers)
		}
	}
	if len(reg.Servers) == 0 {
		return nil
	}
	return w.store.UpsertRemoteServers(ctx, reg)
}
