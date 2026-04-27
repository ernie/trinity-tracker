package domain

import (
	"encoding/json"
	"time"
)

// EnvelopeSchemaVersion is the current wire format version. Bump when
// adding or removing envelope fields in a way that the hub cannot
// tolerate.
const EnvelopeSchemaVersion = 1

// Envelope wraps a FactEvent's payload with the transport-level
// metadata needed for distributed tracking: source identity, monotonic
// sequence, UTC timestamp, and event type. It is the on-wire JSON
// shape published on trinity.events.<source>. Data is the opaque
// payload (the FactEvent.Data marshaled to JSON); the hub decodes it
// against the event-type-specific struct after routing.
//
// Source is the admin-chosen collector identifier. NATS subject
// scoping already binds each authenticated connection to its own
// source, so the hub cross-checks but does not rely on the envelope
// field alone for identity.
type Envelope struct {
	SchemaVersion  int             `json:"v"`
	Source         string          `json:"source"`
	RemoteServerID int64           `json:"rsid"`
	Seq            uint64          `json:"seq"`
	Timestamp      time.Time       `json:"ts"`
	Event          string          `json:"event"`
	Data           json.RawMessage `json:"data"`
}
