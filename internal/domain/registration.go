package domain

// RegistrationSchemaVersion is bumped when the Registration or
// RegdServer wire shape changes in an incompatible way.
const RegistrationSchemaVersion = 1

// Registration is the payload a collector publishes to
// trinity.register.<source> on connect and every heartbeat_interval.
// Its dual purpose is liveness (the hub updates last_heartbeat_at from
// it) and roster delivery (the hub uses it to populate pending_sources
// for first-time collectors and later to pre-populate servers rows on
// admin approval).
type Registration struct {
	Source        string       `json:"source"`
	SourceUUID    string       `json:"source_uuid"`
	Version       string       `json:"version"`
	SchemaVersion int          `json:"schema_version"`
	Servers       []RegdServer `json:"servers"`
}

// RegdServer is one Q3 server entry inside a Registration. LocalID is
// the collector-local identifier (the collector's own servers.id);
// Address is the public host:port the hub uses for UDP status polling.
type RegdServer struct {
	LocalID int64  `json:"local_id"`
	Name    string `json:"name"`
	Address string `json:"address"`
}
