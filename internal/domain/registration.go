package domain

// RegistrationSchemaVersion is bumped when the Registration or
// RegdServer wire shape changes in an incompatible way.
const RegistrationSchemaVersion = 1

// Registration is the payload a collector publishes to
// trinity.register.<source> on connect and every heartbeat_interval.
// It carries liveness (the hub updates last_heartbeat_at) and the
// roster (the hub upserts a servers row per entry, so an operator's
// roster edits land with no admin step). The hub refuses
// registrations from unknown sources.
type Registration struct {
	Source        string       `json:"source"`
	Version       string       `json:"version"`
	SchemaVersion int          `json:"schema_version"`
	DemoBaseURL   string       `json:"demo_base_url,omitempty"`
	Servers       []RegdServer `json:"servers"`
}

// RegdServer is one game server in a Registration. LocalID is the
// collector's own servers.id; Key is the stable identifier from the
// collector's q3_servers config; Address is the public host:port the
// hub polls over UDP.
//
// AdminDelegationEnabled mirrors the operator's per-server opt-in
// (Q3Server.AllowHubAdminRcon), so a config change reaches the hub
// within one heartbeat. The collector re-validates every proxied RCON
// request; the hub uses the value only to gate admin UI.
type RegdServer struct {
	LocalID                int64  `json:"local_id"`
	Key                    string `json:"key"`
	Address                string `json:"address"`
	AdminDelegationEnabled bool   `json:"admin_delegation_enabled,omitempty"`

	// IsLive reports whether a live-stream tap is open, so a viewer can
	// connect to /tv/<source>/<key>. It rides every heartbeat and is also
	// pushed when the tap opens or closes. The hub keeps it in memory only.
	IsLive bool `json:"is_live,omitempty"`

	// LiveDelaySeconds is the collector's target viewer delay (live_delay):
	// encode latency plus relay holdback, in whole seconds, the same for
	// every server on the collector. The live player shows it as the
	// feed's lag. In memory only.
	LiveDelaySeconds int `json:"live_delay_seconds,omitempty"`

	// LiveViewers is how many web viewers the collector's relay is
	// streaming this server to. It updates only with the heartbeat, so
	// viewer churn adds no NATS or database traffic. In memory only.
	LiveViewers int `json:"live_viewers,omitempty"`
}
