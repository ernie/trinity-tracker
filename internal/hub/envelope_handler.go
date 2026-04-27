package hub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// HandleEnvelope is the hub-side entry point for an event that has
// arrived over the wire (distributed mode). It dedups against
// source_progress.consumed_seq, decodes the typed payload, dispatches
// to the existing fact-event handlers, and advances consumed_seq on
// success.
//
// Phase 4 uses env.RemoteServerID directly as the local servers.id —
// correct for hub+collector in one process. Phase 6 will map
// (source_uuid, RemoteServerID) → servers.id via the registration
// table once remote collectors exist.
func (w *Writer) HandleEnvelope(ctx context.Context, env domain.Envelope) error {
	if env.SourceUUID == "" {
		return fmt.Errorf("hub: envelope missing source_uuid (event=%s seq=%d)", env.Event, env.Seq)
	}

	prev, err := w.store.GetConsumedSeq(ctx, env.SourceUUID)
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

	w.dispatch(ctx, domain.FactEvent{
		Type:      env.Event,
		ServerID:  env.RemoteServerID,
		Timestamp: env.Timestamp,
		Data:      payload,
	})

	return w.store.AdvanceConsumedSeq(ctx, env.SourceUUID, env.Seq)
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
