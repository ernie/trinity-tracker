package hub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// HandleEnvelope is the hub-side entry point for an event that has
// arrived over the wire (distributed mode). It dedups against
// source_progress.consumed_seq, resolves the envelope's remote server
// id to a local servers.id (via (source_uuid, local_id)), decodes the
// typed payload, dispatches through the existing fact-event handlers,
// and advances consumed_seq on success.
//
// If the source has no mapping yet (unknown/pending), we still fall
// back to treating RemoteServerID as a literal servers.id. This keeps
// the in-process hub+collector loopback working throughout M2 and
// degrades gracefully when a remote collector publishes events that
// arrive before its approval lands (phase 6b will intercept those
// via the pending DLQ).
func (w *Writer) HandleEnvelope(ctx context.Context, env domain.Envelope) error {
	if env.SourceUUID == "" {
		return fmt.Errorf("hub: envelope missing source_uuid (event=%s seq=%d)", env.Event, env.Seq)
	}

	state, err := w.sources.State(ctx, env.SourceUUID)
	if err != nil {
		return err
	}
	switch state {
	case SourceBlocked:
		return nil
	case SourcePending:
		w.sources.EnqueueDLQ(env.SourceUUID, env)
		return nil
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

	serverID := env.RemoteServerID
	if resolved, err := w.store.ResolveServerIDForSource(ctx, env.SourceUUID, env.RemoteServerID); err == nil && resolved != 0 {
		serverID = resolved
	}

	w.dispatch(ctx, domain.FactEvent{
		Type:      env.Event,
		ServerID:  serverID,
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
