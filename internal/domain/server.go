package domain

import "time"

// Server represents a Quake 3 server being monitored. Identity is
// (source, key) — see schema.sql. The pretty display string the UI
// shows is composed at render time as "<source> / <key>". Active=false
// means the operator has decommissioned the server (cfg removal or
// source deactivation cascade); the row sticks around for historical
// matches and the UI dims it.
type Server struct {
	ID                int64      `json:"id"`
	Source            string     `json:"source"`
	Key               string     `json:"key"`
	Address           string     `json:"address"`
	Active            bool       `json:"active"`
	// HandshakeRequired latches to true the first time the hub sees a
	// match_start with handshake_required=true on this server, and
	// flips back to false on a match_start with handshake_required=false.
	// New rows default to false so the hub rejects everything from the
	// server until the cvar is observed.
	HandshakeRequired bool       `json:"handshake_required"`
	LastMatchUUID     *string    `json:"last_match_uuid,omitempty"`
	LastMatchEndedAt  *time.Time `json:"last_match_ended_at,omitempty"`
	// LastHeartbeatAt mirrors sources.last_heartbeat_at, written by
	// the hub on every collector heartbeat. Drives the "stale" /
	// "hide" rules in the live-cards endpoint — older than the hide
	// threshold means the collector isn't checking in and the data
	// behind the live card is no longer trustworthy.
	LastHeartbeatAt   *time.Time `json:"last_heartbeat_at,omitempty"`
	// AdminDelegationEnabled is the operator's per-server opt-in for
	// hub-admin RCON. Refreshed on every collector heartbeat. The
	// collector stays authoritative — this column drives UI gating
	// only.
	AdminDelegationEnabled bool   `json:"admin_delegation_enabled"`
	CreatedAt         time.Time  `json:"created_at"`
}

// ServerStatus represents the current state of a server from UDP query
type ServerStatus struct {
	ServerID        int64             `json:"server_id"`
	Source          string            `json:"source"`
	Key             string            `json:"key"`
	Address         string            `json:"address"`
	Map             string            `json:"map"`
	GameType        string            `json:"game_type"`
	GameTimeMs      int               `json:"game_time_ms"`
	MaxClients      int               `json:"max_clients"`
	Players         []PlayerStatus    `json:"players"`
	HumanCount      int               `json:"human_count"`
	BotCount        int               `json:"bot_count"`
	Online          bool              `json:"online"`
	LastUpdated     time.Time         `json:"last_updated"`
	// LastSeenAt is the timestamp of the most recent successful UDP
	// query. Held even after the server goes offline so the API can
	// compute "offline duration" from it.
	LastSeenAt      *time.Time        `json:"last_seen_at,omitempty"`
	ServerVars      map[string]string `json:"server_vars,omitempty"`
	TeamScores      *TeamScores       `json:"team_scores,omitempty"`
	FlagStatus      *FlagStatus       `json:"flag_status,omitempty"`
	MatchState      string            `json:"match_state,omitempty"`       // "waiting", "warmup", "active", "overtime", "intermission"
	WarmupRemaining int               `json:"warmup_remaining,omitempty"` // milliseconds remaining in warmup
}

// TeamScores represents team scores for team game modes
type TeamScores struct {
	RedScore  int `json:"red"`
	BlueScore int `json:"blue"`
}

// FlagStatus represents CTF / 1FCTF flag states. Mode disambiguates
// which fields are meaningful. CTF status values: 0=at base, 1=taken,
// 2=dropped. 1FCTF status values (neutral flag only): 0=at base,
// 2=carried by red, 3=carried by blue, 4=dropped. Carrier values are
// client_num of the carrier, or -1 if not carried.
type FlagStatus struct {
	Mode           string `json:"mode,omitempty"` // "ctf" | "1fctf"
	Red            int    `json:"red"`
	RedCarrier     int    `json:"red_carrier"`
	Blue           int    `json:"blue"`
	BlueCarrier    int    `json:"blue_carrier"`
	Neutral        int    `json:"neutral,omitempty"`
	NeutralCarrier int    `json:"neutral_carrier,omitempty"`
}

// PlayerStatus represents a player's current state on a server
type PlayerStatus struct {
	ClientNum    int       `json:"client_num"`
	GUID         string    `json:"guid,omitempty"`
	Name         string    `json:"name"`
	CleanName    string    `json:"clean_name"`
	Score        int       `json:"score"`
	Ping         int       `json:"ping"`
	IsBot        bool      `json:"is_bot"`
	IsVR         bool      `json:"is_vr"`
	Skill        float64   `json:"skill,omitempty"`        // bot skill level (1-5), 0 if human
	Team         int       `json:"team,omitempty"`
	JoinedAt     time.Time `json:"joined_at,omitempty"`
	Impressives  int       `json:"impressives,omitempty"`  // impressive awards this match
	Excellents   int       `json:"excellents,omitempty"`   // excellent awards this match
	Humiliations int       `json:"humiliations,omitempty"` // gauntlet kills this match
	Defends      int       `json:"defends,omitempty"`      // defend awards this match
	Captures     int       `json:"captures,omitempty"`     // flag captures this match
	Assists      int       `json:"assists,omitempty"`      // assist awards this match
	PlayerID     *int64    `json:"player_id,omitempty"`    // database player ID if known
	IsVerified   bool      `json:"is_verified"`
	IsAdmin      bool      `json:"is_admin"`
	Model        string    `json:"model,omitempty"`        // player model (e.g., "sarge/krusade")
}
