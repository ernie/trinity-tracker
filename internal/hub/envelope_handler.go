package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// HandleEnvelope ingests an event from the bus: it dedups against
// source_progress.consumed_seq, resolves the server from (source,
// local_id), decodes and dispatches the payload, and advances
// consumed_seq.
//
// Dedup uses only the seq, because game log timestamps aren't monotonic
// within a second. A reinstalled collector stays above the hub's
// watermark by seeding its first seq from the hub. last_consumed_ts is
// for monitoring.
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
		// NATS auth should have refused this at the broker, so auth is
		// misconfigured or the source was deleted after its credentials
		// were issued.
		log.Printf("hub: envelope from unprovisioned source=%q event=%s seq=%d; dropping", env.Source, env.Event, env.Seq)
		return nil
	}

	prog, err := w.store.GetSourceProgress(ctx, env.Source)
	if err != nil {
		return err
	}
	if env.Seq <= prog.ConsumedSeq {
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

	return w.store.AdvanceSourceProgress(ctx, env.Source, env.Seq, env.Timestamp.Truncate(time.Second).UTC())
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
	case domain.FactPlayerVR:
		var p domain.PlayerVRData
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
	case domain.FactDemoFinalized:
		var p domain.DemoFinalizedData
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("hub: decode %s: %w", event, err)
		}
		return p, nil
	case domain.FactRconExec:
		var p domain.RconExecData
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("hub: decode %s: %w", event, err)
		}
		return p, nil
	case domain.FactRconDenied:
		var p domain.RconDeniedData
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("hub: decode %s: %w", event, err)
		}
		return p, nil
	default:
		return nil, fmt.Errorf("hub: unknown event type %q", event)
	}
}
