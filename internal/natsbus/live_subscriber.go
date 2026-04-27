package natsbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/hub"
)

// LiveServerResolver maps (source, remote_server_id) to a local
// servers.id for live-event dispatch.
type LiveServerResolver interface {
	ResolveServerIDForSource(ctx context.Context, source string, remoteServerID int64) (int64, error)
}

// LiveEventEnricher fills in PlayerID fields on live-event payloads.
type LiveEventEnricher interface {
	EnrichEvent(ctx context.Context, event domain.Event) domain.Event
}

// LiveSubscriber pushes trinity.live.> envelopes (decoded + enriched)
// onto the sink. Skips events matching selfSource so a co-located
// collector's own events aren't duplicated.
type LiveSubscriber struct {
	nc         *nats.Conn
	sub        *nats.Subscription
	resolver   LiveServerResolver
	enricher   LiveEventEnricher
	sink       hub.LiveEventSink
	selfSource string
}

// NewLiveSubscriber subscribes to trinity.live.>. Pass the local
// collector's source to skip its own events, or "" if none.
func NewLiveSubscriber(nc *nats.Conn, resolver LiveServerResolver, enricher LiveEventEnricher, sink hub.LiveEventSink, selfSource string) (*LiveSubscriber, error) {
	if nc == nil {
		return nil, fmt.Errorf("natsbus.NewLiveSubscriber: NATS connection is required")
	}
	if resolver == nil {
		return nil, fmt.Errorf("natsbus.NewLiveSubscriber: resolver is required")
	}
	if sink == nil {
		return nil, fmt.Errorf("natsbus.NewLiveSubscriber: sink is required")
	}
	s := &LiveSubscriber{
		nc:         nc,
		resolver:   resolver,
		enricher:   enricher,
		sink:       sink,
		selfSource: selfSource,
	}
	sub, err := nc.Subscribe(SubjectLivePrefix+">", s.handle)
	if err != nil {
		return nil, fmt.Errorf("natsbus.NewLiveSubscriber: subscribe: %w", err)
	}
	s.sub = sub
	return s, nil
}

func (s *LiveSubscriber) Stop() {
	if s == nil || s.sub == nil {
		return
	}
	_ = s.sub.Unsubscribe()
	s.sub = nil
}

func (s *LiveSubscriber) handle(m *nats.Msg) {
	var env domain.Envelope
	if err := json.Unmarshal(m.Data, &env); err != nil {
		log.Printf("natsbus.LiveSubscriber: discarding unparseable envelope: %v", err)
		return
	}
	if env.Source == "" {
		log.Printf("natsbus.LiveSubscriber: envelope missing source (event=%s)", env.Event)
		return
	}
	if s.selfSource != "" && env.Source == s.selfSource {
		return // already delivered via the in-process manager.Events() channel
	}

	ctx := context.Background()
	serverID := env.RemoteServerID
	if resolved, err := s.resolver.ResolveServerIDForSource(ctx, env.Source, env.RemoteServerID); err == nil && resolved != 0 {
		serverID = resolved
	}

	payload, err := decodeLiveEventPayload(env.Event, env.Data)
	if err != nil {
		log.Printf("natsbus.LiveSubscriber: decode %s for source %q: %v", env.Event, env.Source, err)
		return
	}

	event := domain.Event{
		Type:      env.Event,
		ServerID:  serverID,
		Timestamp: env.Timestamp,
		Data:      payload,
	}
	if s.enricher != nil {
		event = s.enricher.EnrichEvent(ctx, event)
	}
	s.sink.Broadcast(event)
}

func decodeLiveEventPayload(eventType string, raw json.RawMessage) (interface{}, error) {
	var target interface{}
	switch eventType {
	case domain.EventPlayerJoin:
		target = &domain.PlayerJoinEvent{}
	case domain.EventPlayerLeave:
		target = &domain.PlayerLeaveEvent{}
	case domain.EventMatchStart:
		target = &domain.MatchStartEvent{}
	case domain.EventMatchEnd:
		target = &domain.MatchEndEvent{}
	case domain.EventFrag:
		target = &domain.FragEvent{}
	case domain.EventFlagCapture:
		target = &domain.FlagCaptureEvent{}
	case domain.EventFlagTaken:
		target = &domain.FlagTakenEvent{}
	case domain.EventFlagReturn:
		target = &domain.FlagReturnEvent{}
	case domain.EventFlagDrop:
		target = &domain.FlagDropEvent{}
	case domain.EventObeliskDestroy:
		target = &domain.ObeliskDestroyEvent{}
	case domain.EventSkullScore:
		target = &domain.SkullScoreEvent{}
	case domain.EventTeamChange:
		target = &domain.TeamChangeEvent{}
	case domain.EventSay:
		target = &domain.SayEvent{}
	case domain.EventSayTeam:
		target = &domain.SayTeamEvent{}
	case domain.EventTell:
		target = &domain.TellEvent{}
	case domain.EventSayRcon:
		target = &domain.SayRconEvent{}
	case domain.EventAward:
		target = &domain.AwardEvent{}
	case domain.EventClientUserinfo:
		target = &domain.ClientUserinfoEvent{}
	default:
		return nil, fmt.Errorf("unknown live event type %q", eventType)
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, target); err != nil {
			return nil, err
		}
	}
	// Return a value (not a pointer) so EnrichEvent's type switch matches.
	switch t := target.(type) {
	case *domain.PlayerJoinEvent:
		return *t, nil
	case *domain.PlayerLeaveEvent:
		return *t, nil
	case *domain.MatchStartEvent:
		return *t, nil
	case *domain.MatchEndEvent:
		return *t, nil
	case *domain.FragEvent:
		return *t, nil
	case *domain.FlagCaptureEvent:
		return *t, nil
	case *domain.FlagTakenEvent:
		return *t, nil
	case *domain.FlagReturnEvent:
		return *t, nil
	case *domain.FlagDropEvent:
		return *t, nil
	case *domain.ObeliskDestroyEvent:
		return *t, nil
	case *domain.SkullScoreEvent:
		return *t, nil
	case *domain.TeamChangeEvent:
		return *t, nil
	case *domain.SayEvent:
		return *t, nil
	case *domain.SayTeamEvent:
		return *t, nil
	case *domain.TellEvent:
		return *t, nil
	case *domain.SayRconEvent:
		return *t, nil
	case *domain.AwardEvent:
		return *t, nil
	case *domain.ClientUserinfoEvent:
		return *t, nil
	}
	return nil, fmt.Errorf("unreachable: live event %q unmapped after decode", eventType)
}
