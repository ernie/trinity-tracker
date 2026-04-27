package hub

import (
	"context"
	"fmt"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// HandleRegistration is invoked for each incoming trinity.register.*
// message. Known source_uuids refresh last_heartbeat_at; unknown ones
// upsert a pending_sources row for admin review. The admin flow (phase
// 11) will approve them into real servers rows.
func (w *Writer) HandleRegistration(ctx context.Context, reg domain.Registration) error {
	if reg.SourceUUID == "" {
		return fmt.Errorf("hub.registration: missing source_uuid (source=%q)", reg.Source)
	}
	approved, err := w.store.IsSourceApproved(ctx, reg.SourceUUID)
	if err != nil {
		return err
	}
	if approved {
		return w.store.TouchSourceHeartbeat(ctx, reg.SourceUUID, time.Now().UTC())
	}
	return w.store.UpsertPendingSource(ctx, reg)
}
