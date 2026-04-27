package hub

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// HandleRegistration processes a heartbeat. Unknown sources are
// refused (NATS auth should have blocked them already; this is
// defense-in-depth plus the one-line operator signal when auth is
// misconfigured). Known sources touch their heartbeat and upsert the
// server roster so admins see operator-side roster edits without
// manual intervention.
func (w *Writer) HandleRegistration(ctx context.Context, reg domain.Registration) error {
	if reg.Source == "" {
		return fmt.Errorf("hub.registration: missing source")
	}
	approved, err := w.store.IsSourceApproved(ctx, reg.Source)
	if err != nil {
		return err
	}
	if !approved {
		log.Printf("hub: registration from unprovisioned source=%q — refusing; admin must create the source first", reg.Source)
		return nil
	}
	if err := w.store.TouchSourceHeartbeat(ctx, reg.Source, time.Now().UTC(), reg.Version, reg.DemoBaseURL); err != nil {
		return err
	}
	if len(reg.Servers) == 0 {
		return nil
	}
	return w.store.UpsertRemoteServers(ctx, reg)
}
