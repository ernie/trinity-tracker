package collector

import (
	"context"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/hub"
)

type vrTestServer struct{ hub.ServerClient }

func (vrTestServer) UpsertPlayerIdentity(ctx context.Context, guid, name, cleanName string, ts time.Time, isVR bool) (hub.PlayerIdentity, error) {
	return hub.PlayerIdentity{Found: true}, nil
}

type vrTestPublisher struct{ facts []domain.FactEvent }

func (p *vrTestPublisher) Publish(e domain.FactEvent) error {
	p.facts = append(p.facts, e)
	return nil
}

func newVRTestManager(isVR bool) (*ServerManager, *serverState, *vrTestPublisher) {
	state := &serverState{
		matchState: "active",
		clients: map[int]*clientState{
			4: {clientID: 4, name: "Player", cleanName: "Player", guid: "GUID4", team: 0, began: true, isVR: isVR, vrSeen: isVR},
		},
	}
	pub := &vrTestPublisher{}
	m := &ServerManager{
		events:  make(chan domain.Event, 8),
		servers: map[int64]*serverState{7: state},
		server:  vrTestServer{},
		pub:     pub,
	}
	return m, state, pub
}

func userinfoEvent(isVR bool) LogEvent {
	return LogEvent{
		Timestamp: time.Date(2026, 10, 9, 21, 0, 0, 0, time.UTC),
		Type:      EventTypeClientUserinfo,
		Data:      ClientUserinfoData{ClientID: 4, Name: "Player", GUID: "GUID4", IsVR: isVR, Userinfo: map[string]string{}},
	}
}

func playerVRFacts(pub *vrTestPublisher) []domain.PlayerVRData {
	var out []domain.PlayerVRData
	for _, f := range pub.facts {
		if f.Type == domain.FactPlayerVR {
			out = append(out, f.Data.(domain.PlayerVRData))
		}
	}
	return out
}

func TestSwitchToVRPublishesPlayerVR(t *testing.T) {
	m, _, pub := newVRTestManager(false)
	m.handleLogEvent(context.Background(), 7, userinfoEvent(true), false)
	got := playerVRFacts(pub)
	if len(got) != 1 || got[0].GUID != "GUID4" {
		t.Fatalf("player_vr facts = %+v, want one for GUID4", got)
	}
}

func TestStayingInVRPublishesNothing(t *testing.T) {
	m, _, pub := newVRTestManager(true)
	m.handleLogEvent(context.Background(), 7, userinfoEvent(true), false)
	if got := playerVRFacts(pub); len(got) != 0 {
		t.Fatalf("player_vr facts = %+v, want none", got)
	}
}

func TestSwitchToFlatKeepsMatchVR(t *testing.T) {
	m, state, _ := newVRTestManager(true)
	m.handleLogEvent(context.Background(), 7, userinfoEvent(false), false)
	players := m.buildMatchEndPlayers(state, false)
	if len(players) != 1 || !players[0].IsVR {
		t.Fatalf("match end players = %+v, want GUID4 recorded as VR", players)
	}
}

func TestPreviousStintKeepsMatchVR(t *testing.T) {
	state := &serverState{}
	state.savePreviousClient(&clientState{guid: "GUID4", vrSeen: true})
	state.savePreviousClient(&clientState{guid: "GUID4"})
	if !state.previousClients["GUID4"].vrSeen {
		t.Error("a later flat stint cleared the match's VR flag")
	}
}
