package domain

import (
	"encoding/json"
	"time"
)

// EnvelopeSchemaVersion is the current wire format version. Bump it
// for any envelope change the hub can't tolerate.
const EnvelopeSchemaVersion = 1

// Envelope is the JSON published on trinity.events.<source>: a
// FactEvent's payload plus source, sequence, UTC timestamp and event
// type. Data is the FactEvent.Data marshaled to JSON; the hub decodes
// it against the event's own struct after routing.
//
// NATS subject scoping already binds each connection to its source,
// so the hub only cross-checks the Source field.
type Envelope struct {
	SchemaVersion  int             `json:"v"`
	Source         string          `json:"source"`
	RemoteServerID int64           `json:"rsid"`
	Seq            uint64          `json:"seq"`
	Timestamp      time.Time       `json:"ts"`
	Event          string          `json:"event"`
	Data           json.RawMessage `json:"data"`
}
