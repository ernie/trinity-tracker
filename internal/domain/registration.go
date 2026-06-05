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
//
// AdminDelegationEnabled mirrors the operator's per-server opt-in
// (Q3Server.AllowHubAdminRcon). It travels with every heartbeat so a
// flip in cfg propagates within one heartbeat interval — the collector
// stays authoritative and re-validates on each proxied RCON request,
// but the hub uses the value to gate UI affordances for hub admins.
type RegdServer struct {
	LocalID                int64  `json:"local_id"`
	Key                    string `json:"key"`
	Address                string `json:"address"`
	AdminDelegationEnabled bool   `json:"admin_delegation_enabled,omitempty"`

	// IsLive reports whether this server currently has an active live-stream tap
	// (a viewer can connect to /tv/<source>/<key>). It rides every heartbeat
	// and is pushed out-of-band on tap open/close. Ephemeral match state, not
	// roster config — the hub keeps it in memory only.
	IsLive bool `json:"is_live,omitempty"`

	// LiveDelaySeconds is the collector's target viewer delay (live_delay), the
	// encode latency + relay holdback, in whole seconds. The same for every server
	// a collector serves; it rides the heartbeat so the live player can honestly
	// show the feed's lag. In memory only, like IsLive.
	LiveDelaySeconds int `json:"live_delay_seconds,omitempty"`
}
