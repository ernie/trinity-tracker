package domain

// RegistrationSchemaVersion is bumped when the Registration or
// RegdServer wire shape changes in an incompatible way.
const RegistrationSchemaVersion = 1

// Registration is the payload a collector publishes to
// trinity.register.<source> on connect and every heartbeat_interval.
// Its dual purpose is liveness (the hub updates last_heartbeat_at from
// it) and roster delivery (the hub upserts servers rows for every
// entry each heartbeat, so operator-side roster edits land without an
// admin round-trip). Registrations from unprovisioned source_uuids
// are refused.
type Registration struct {
	Source        string       `json:"source"`
	Version       string       `json:"version"`
	SchemaVersion int          `json:"schema_version"`
	DemoBaseURL   string       `json:"demo_base_url,omitempty"`
	Servers       []RegdServer `json:"servers"`
}

// RegdServer is one Q3 server entry inside a Registration. LocalID is
// the collector-local identifier (the collector's own servers.id);
// Key is the stable identifier from the collector's q3_servers cfg;
// Address is the public host:port the hub uses for UDP status polling.
type RegdServer struct {
	LocalID int64  `json:"local_id"`
	Key     string `json:"key"`
	Address string `json:"address"`
}
