package collector

import (
	"context"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

func newAssistTestManager(matchState string) (*ServerManager, *serverState) {
	state := &serverState{
		matchState: matchState,
		clients: map[int]*clientState{
			6: {clientID: 6, name: "Visor", guid: "GUID6", team: 2},
		},
	}
	m := &ServerManager{
		events:  make(chan domain.Event, 1),
		servers: map[int64]*serverState{7: state},
	}
	return m, state
}

func assistLogEvent(assistType string) LogEvent {
	return LogEvent{
		Timestamp: time.Date(2026, 6, 12, 21, 0, 0, 0, time.UTC),
		Type:      EventTypeAssist,
		Data: AssistData{
			ClientID:   6,
			Team:       2,
			AssistType: assistType,
			Name:       "Visor",
		},
	}
}

func TestAssistEmitsLiveAwardEvent(t *testing.T) {
	m, state := newAssistTestManager("active")

	m.handleLogEvent(context.Background(), 7, assistLogEvent("skull"), false)

	if got := state.clients[6].assists; got != 1 {
		t.Errorf("assists = %d, want 1", got)
	}
	select {
	case ev := <-m.events:
		if ev.Type != domain.EventAward {
			t.Fatalf("event type = %q, want %q", ev.Type, domain.EventAward)
		}
		d := ev.Data.(domain.AwardEvent)
		want := domain.AwardEvent{
			ClientNum:  6,
			PlayerName: "Visor",
			AwardType:  "assist",
			AssistType: "skull",
			Team:       2,
			GUID:       "GUID6",
		}
		if d != want {
			t.Errorf("AwardEvent = %+v, want %+v", d, want)
		}
	default:
		t.Fatal("no live event emitted for assist")
	}
}

func TestAssistIgnoredOutsideActiveMatch(t *testing.T) {
	m, state := newAssistTestManager("warmup")

	m.handleLogEvent(context.Background(), 7, assistLogEvent("return"), false)

	if got := state.clients[6].assists; got != 0 {
		t.Errorf("assists = %d, want 0", got)
	}
	select {
	case ev := <-m.events:
		t.Fatalf("unexpected live event %q during warmup", ev.Type)
	default:
	}
}

func TestAssistReplayCountsWithoutLiveEvent(t *testing.T) {
	m, state := newAssistTestManager("active")

	m.handleLogEvent(context.Background(), 7, assistLogEvent("frag"), true)

	if got := state.clients[6].assists; got != 1 {
		t.Errorf("assists = %d, want 1", got)
	}
	select {
	case ev := <-m.events:
		t.Fatalf("unexpected live event %q in replay mode", ev.Type)
	default:
	}
}
