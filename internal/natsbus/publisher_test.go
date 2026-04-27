package natsbus

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/domain"
)

func connectInProcess(t *testing.T, s *Server) *nats.Conn {
	t.Helper()
	nc, err := s.ConnectInternal()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(nc.Close)
	return nc
}

func TestPublisherRoundTrip(t *testing.T) {
	s := startTestServer(t)
	nc := connectInProcess(t, s)
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	sub, err := js.PullSubscribe("trinity.events.remote-1", "test-consumer")
	if err != nil {
		t.Fatalf("PullSubscribe: %v", err)
	}

	pub, err := NewPublisher(nc, "remote-1", 0)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	ts := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	payload := domain.MatchStartData{MatchUUID: "m-1", MapName: "q3dm17", GameType: "FFA", StartedAt: ts}
	if err := pub.Publish(domain.FactEvent{
		Type:      domain.FactMatchStart,
		ServerID:  7,
		Timestamp: ts,
		Data:      payload,
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	msgs, err := sub.Fetch(1, nats.MaxWait(5*time.Second))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d msgs, want 1", len(msgs))
	}
	msg := msgs[0]
	defer msg.Ack()

	if got := msg.Subject; got != "trinity.events.remote-1" {
		t.Errorf("subject = %q", got)
	}
	if got := msg.Header.Get(nats.MsgIdHdr); got != "remote-1:1" {
		t.Errorf("Nats-Msg-Id = %q, want remote-1:1", got)
	}

	var env domain.Envelope
	if err := json.Unmarshal(msg.Data, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.SchemaVersion != domain.EnvelopeSchemaVersion {
		t.Errorf("SchemaVersion = %d", env.SchemaVersion)
	}
	if env.Seq != 1 || env.Source != "remote-1" || env.Event != domain.FactMatchStart {
		t.Errorf("envelope metadata wrong: %+v", env)
	}
	if env.RemoteServerID != 7 {
		t.Errorf("RemoteServerID = %d, want 7", env.RemoteServerID)
	}
	if env.Timestamp.Location() != time.UTC {
		t.Errorf("Timestamp not UTC: %v", env.Timestamp)
	}
	var decoded domain.MatchStartData
	if err := json.Unmarshal(env.Data, &decoded); err != nil {
		t.Fatalf("inner payload: %v", err)
	}
	if decoded.MatchUUID != "m-1" || decoded.MapName != "q3dm17" {
		t.Errorf("payload = %+v", decoded)
	}
}

func TestPublisherSeqMonotonic(t *testing.T) {
	s := startTestServer(t)
	nc := connectInProcess(t, s)
	pub, err := NewPublisher(nc, "x", 100)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := pub.Publish(domain.FactEvent{
			Type:      domain.FactPlayerJoin,
			Timestamp: time.Now().UTC(),
			Data:      domain.PlayerJoinData{GUID: "G", Name: "n"},
		}); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}
	if got := pub.LastSeq(); got != 103 {
		t.Errorf("LastSeq = %d, want 103 (100 + 3)", got)
	}
}

func TestNewPublisherValidation(t *testing.T) {
	s := startTestServer(t)
	nc := connectInProcess(t, s)
	if _, err := NewPublisher(nil, "a", 0); err == nil {
		t.Error("expected error for nil conn")
	}
	if _, err := NewPublisher(nc, "", 0); err == nil {
		t.Error("expected error for empty source")
	}
}
