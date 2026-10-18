package domain

import "time"

// Server is a monitored game server, identified by (source, key); the
// UI shows "<source> / <key>". Active is false once the operator
// removes the server from config or its source is deactivated; the row
// stays for its matches and the UI dims it.
type Server struct {
	ID      int64  `json:"id"`
	Source  string `json:"source"`
	Key     string `json:"key"`
	Address string `json:"address"`
	Active  bool   `json:"active"`
	// HandshakeRequired follows the handshake_required flag of the
	// server's latest match_start. New rows start false, so the hub
	// rejects the server's events until a match_start sets it.
	HandshakeRequired bool       `json:"handshake_required"`
	LastMatchUUID     *string    `json:"last_match_uuid,omitempty"`
	LastMatchEndedAt  *time.Time `json:"last_match_ended_at,omitempty"`
	// LastHeartbeatAt mirrors sources.last_heartbeat_at. The live-cards
	// endpoint marks a card stale, then hides it, as this ages.
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at,omitempty"`
	// AdminDelegationEnabled is the operator's per-server opt-in for
	// hub-admin RCON, refreshed every heartbeat. The collector enforces
	// it; here it only gates UI.
	AdminDelegationEnabled bool      `json:"admin_delegation_enabled"`
	CreatedAt              time.Time `json:"created_at"`
}

// ServerStatus is a server's current state from its UDP status query.
type ServerStatus struct {
	ServerID    int64          `json:"server_id"`
	Source      string         `json:"source"`
	Key         string         `json:"key"`
	Address     string         `json:"address"`
	Map         string         `json:"map"`
	GameType    string         `json:"game_type"`
	GameTimeMs  int            `json:"game_time_ms"`
	MaxClients  int            `json:"max_clients"`
	Players     []PlayerStatus `json:"players"`
	HumanCount  int            `json:"human_count"`
	BotCount    int            `json:"bot_count"`
	Online      bool           `json:"online"`
	LastUpdated time.Time      `json:"last_updated"`
	// LastSeenAt is the last successful UDP query. It survives the
	// server going offline so the API can report how long it's been down.
	LastSeenAt       *time.Time        `json:"last_seen_at,omitempty"`
	ServerVars       map[string]string `json:"server_vars,omitempty"`
	TeamScores       *TeamScores       `json:"team_scores,omitempty"`
	ObjStatus        *ObjStatus        `json:"obj_status,omitempty"`
	ObeliskHealthMax int               `json:"obelisk_health_max,omitempty"` // g_obeliskHealth; HP-bar denominator for Overload
	MatchState       string            `json:"match_state,omitempty"`        // "waiting", "warmup", "active", "overtime", "intermission"
	WarmupRemaining  int               `json:"warmup_remaining,omitempty"`   // milliseconds remaining in warmup
	IsLive           bool              `json:"is_live"`                      // a live-stream tap is active; /tv/<source>/<key> is connectable
	LiveDelaySeconds int               `json:"live_delay_seconds,omitempty"` // collector's target viewer delay (live_delay): encode + holdback, whole seconds; 0 when unknown
	LiveViewers      int               `json:"live_viewers,omitempty"`       // web viewers on the collector's relay; heartbeat-fresh (~30s), like is_live
}

// TeamScores holds the red and blue scores in team game types.
type TeamScores struct {
	RedScore  int `json:"red"`
	BlueScore int `json:"blue"`
}

// ObjStatus is the parsed g_objStatus cvar; Mode says which fields are
// set. The mod defines the format where it registers the cvar in
// g_main.c.
type ObjStatus struct {
	Mode string `json:"mode,omitempty"` // "ctf" | "1fctf" | "overload" | "harvester"
	// Zero is meaningful in all of these: carrier client 0, a flag at
	// base, a destroyed obelisk.
	Red            int `json:"red"`
	RedCarrier     int `json:"red_carrier"`
	Blue           int `json:"blue"`
	BlueCarrier    int `json:"blue_carrier"`
	Neutral        int `json:"neutral"`
	NeutralCarrier int `json:"neutral_carrier"`
	RedObeliskHP   int `json:"red_obelisk_hp"`
	BlueObeliskHP  int `json:"blue_obelisk_hp"`
	RedSkulls      int `json:"red_skulls,omitempty"`
	BlueSkulls     int `json:"blue_skulls,omitempty"`
}

// PlayerStatus is a player's current state on a server. Award and
// objective counts are for the current match.
type PlayerStatus struct {
	ClientNum         int       `json:"client_num"`
	GUID              string    `json:"guid,omitempty"`
	Name              string    `json:"name"`
	CleanName         string    `json:"clean_name"`
	Score             int       `json:"score"`
	Ping              int       `json:"ping"`
	IsBot             bool      `json:"is_bot"`
	IsVR              bool      `json:"is_vr"`
	Skill             float64   `json:"skill,omitempty"` // bot skill 1-5, 0 for humans
	Team              int       `json:"team,omitempty"`
	JoinedAt          time.Time `json:"joined_at,omitempty"`
	Impressives       int       `json:"impressives,omitempty"`
	Excellents        int       `json:"excellents,omitempty"`
	Humiliations      int       `json:"humiliations,omitempty"`
	Defends           int       `json:"defends,omitempty"`
	Captures          int       `json:"captures,omitempty"`
	Assists           int       `json:"assists,omitempty"`
	FlagReturns       int       `json:"flag_returns,omitempty"`       // player-initiated returns only
	SkullsDelivered   int       `json:"skulls_delivered,omitempty"`   // from Presence
	ObelisksDestroyed int       `json:"obelisks_destroyed,omitempty"` // from the g_objStatus tail
	SkullsCarrying    int       `json:"skulls_carrying,omitempty"`
	PlayerID          *int64    `json:"player_id,omitempty"`
	IsVerified        bool      `json:"is_verified"`
	IsAdmin           bool      `json:"is_admin"`
	Model             string    `json:"model,omitempty"` // "model/skin", such as "sarge/krusade"
}
