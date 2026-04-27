package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// HandleEnvelope ingests a wire-arrived event: dedups against
// source_progress.consumed_seq, resolves RemoteServerID via
// (source, local_id), decodes the payload, dispatches, and
// advances consumed_seq.
func (w *Writer) HandleEnvelope(ctx context.Context, env domain.Envelope) error {
	if env.Source == "" {
		return fmt.Errorf("hub: envelope missing source (event=%s seq=%d)", env.Event, env.Seq)
	}

	state, err := w.sources.State(ctx, env.Source)
	if err != nil {
		return err
	}
	switch state {
	case SourceBlocked:
		return nil
	case SourceUnknown:
		// NATS auth should have rejected this at the broker. If we see
		// one, either auth is misconfigured or creds were issued for a
		// source that was since deleted — either way, drop the event
		// and log once per UnknownCheckTTL window.
		log.Printf("hub: envelope from unprovisioned source=%q event=%s seq=%d — dropping", env.Source, env.Event, env.Seq)
		return nil
	}

	prev, err := w.store.GetConsumedSeq(ctx, env.Source)
	if err != nil {
		return err
	}
	if env.Seq <= prev {
		return nil
	}

	payload, err := decodeEventPayload(env.Event, env.Data)
	if err != nil {
		return err
	}

	serverID := env.RemoteServerID
	if resolved, err := w.store.ResolveServerIDForSource(ctx, env.Source, env.RemoteServerID); err == nil && resolved != 0 {
		serverID = resolved
	}

	w.dispatch(ctx, domain.FactEvent{
		Type:      env.Event,
		ServerID:  serverID,
		Timestamp: env.Timestamp,
		Data:      payload,
	})

	return w.store.AdvanceConsumedSeq(ctx, env.Source, env.Seq)
}

func decodeEventPayload(event string, raw json.RawMessage) (interface{}, error) {
	switch event {
	case domain.FactMatchStart:
		var p domain.MatchStartData
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("hub: decode %s: %w", event, err)
		}
		return p, nil
	case domain.FactMatchEnd:
		var p domain.MatchEndData
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("hub: decode %s: %w", event, err)
		}
		return p, nil
	case domain.FactMatchSettingsUpdate:
		var p domain.MatchSettingsUpdateData
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("hub: decode %s: %w", event, err)
		}
		return p, nil
	case domain.FactMatchCrashed:
		var p domain.MatchCrashedData
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("hub: decode %s: %w", event, err)
		}
		return p, nil
	case domain.FactPlayerJoin:
		var p domain.PlayerJoinData
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("hub: decode %s: %w", event, err)
		}
		return p, nil
	case domain.FactPlayerLeave:
		var p domain.PlayerLeaveData
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("hub: decode %s: %w", event, err)
		}
		return p, nil
	case domain.FactPresenceSnapshot:
		var p domain.PresenceSnapshotData
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("hub: decode %s: %w", event, err)
		}
		return p, nil
	case domain.FactTrinityHandshake:
		var p domain.TrinityHandshakeData
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("hub: decode %s: %w", event, err)
		}
		return p, nil
	case domain.FactServerStartup:
		var p domain.ServerStartupData
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("hub: decode %s: %w", event, err)
		}
		return p, nil
	case domain.FactServerShutdown:
		var p domain.ServerShutdownData
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("hub: decode %s: %w", event, err)
		}
		return p, nil
	default:
		return nil, fmt.Errorf("hub: unknown event type %q", event)
	}
}
