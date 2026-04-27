package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEnvelopeRoundTripPreservesRawData(t *testing.T) {
	payload := MatchStartData{
		MatchUUID: "abc-123",
		MapName:   "q3dm17",
		GameType:  "FFA",
		StartedAt: time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	env := Envelope{
		SchemaVersion:  EnvelopeSchemaVersion,
		Source:         "remote-1",
		RemoteServerID: 7,
		Seq:            42,
		Timestamp:      time.Date(2026, 4, 19, 12, 0, 1, 0, time.UTC),
		Event:          "match_start",
		Data:           payloadJSON,
	}

	body, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	if !strings.Contains(string(body), `"v":1`) {
		t.Errorf("marshal missing v field: %s", body)
	}

	var decoded Envelope
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if decoded.Seq != 42 || decoded.Source != "remote-1" {
		t.Errorf("decoded = %+v", decoded)
	}

	var payloadOut MatchStartData
	if err := json.Unmarshal(decoded.Data, &payloadOut); err != nil {
		t.Fatalf("unmarshal inner payload: %v", err)
	}
	if payloadOut.MatchUUID != payload.MatchUUID || payloadOut.MapName != payload.MapName {
		t.Errorf("inner payload lost fields: %+v", payloadOut)
	}
}
