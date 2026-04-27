package domain

import "time"

// Fact events flow from the collector to the hub writer and drive
// authoritative database writes. Wire identity rule: payloads carry
// GUIDs and match UUIDs, never DB row IDs.

const (
	FactMatchStart           = "match_start"
	FactMatchEnd             = "match_end"
	FactMatchSettingsUpdate  = "match_settings_update"
	FactMatchCrashed         = "match_crashed"
	FactPlayerJoin           = "player_join"
	FactPlayerLeave          = "player_leave"
	FactPresenceSnapshot     = "presence_snapshot"
	FactTrinityHandshake     = "trinity_handshake"
	FactServerStartup        = "server_startup"
	FactServerShutdown       = "server_shutdown"
	FactDemoFinalized        = "demo_finalized"
)

// FactEvent is the in-process envelope carrying a payload from the
// collector to the hub writer. The NATS wire envelope wraps this with
// source/seq/schema_version fields.
type FactEvent struct {
	Type      string      `json:"type"`
	ServerID  int64       `json:"server_id"`
	Timestamp time.Time   `json:"ts"`
	Data      interface{} `json:"data"`
}

// MatchStartData is emitted on WarmupEnd. MatchUUID is assigned by the
// collector and is the key for subsequent match events.
//
// HandshakeRequired reflects the server's g_trinityHandshake cvar. The
// collector refuses to publish match_start when false; the hub double-
// checks. This gates stats to clients that submit a Trinity handshake.
// Trinity servers typically don't run in pure mode because VR clients
// load the game module as a dll/so rather than a QVM, so sv_pure isn't
// available to keep vanilla ioquake3 clients out; g_trinityHandshake is
// the practical substitute. Whether the handshake carries valid auth
// info is independent; the gate fires on handshake presence, not auth.
type MatchStartData struct {
	MatchUUID         string    `json:"match_uuid"`
	MapName           string    `json:"map"`
	GameType          string    `json:"gametype"`
	Movement          string    `json:"movement,omitempty"`
	Gameplay          string    `json:"gameplay,omitempty"`
	StartedAt         time.Time `json:"started_at"`
	HandshakeRequired bool      `json:"handshake_required"`
}

// MatchEndData is emitted on match shutdown (intermission or abrupt).
// Players carries final stats for every participant.
type MatchEndData struct {
	MatchUUID  string           `json:"match_uuid"`
	EndedAt    time.Time        `json:"ended_at"`
	ExitReason string           `json:"exit_reason,omitempty"`
	RedScore   *int             `json:"red_score,omitempty"`
	BlueScore  *int             `json:"blue_score,omitempty"`
	Players    []MatchEndPlayer `json:"players"`
}

// MatchEndPlayer carries one player's final stats for a match. Identity
// is by GUID; the hub writer resolves it to player_guid_id.
type MatchEndPlayer struct {
	GUID         string    `json:"guid"`
	ClientID     int       `json:"client_id"`
	Name         string    `json:"name"`
	CleanName    string    `json:"clean_name"`
	Frags        int       `json:"frags"`
	Deaths       int       `json:"deaths"`
	Completed    bool      `json:"completed"`
	Score        *int      `json:"score,omitempty"`
	Team         *int      `json:"team,omitempty"`
	Model        string    `json:"model,omitempty"`
	Skill        float64   `json:"skill,omitempty"`
	Victory      bool      `json:"victory"`
	Captures     int       `json:"captures"`
	FlagReturns  int       `json:"flag_returns"`
	Assists      int       `json:"assists"`
	Impressives  int       `json:"impressives"`
	Excellents   int       `json:"excellents"`
	Humiliations int       `json:"humiliations"`
	Defends      int       `json:"defends"`
	IsBot        bool      `json:"is_bot"`
	JoinedLate   bool      `json:"joined_late"`
	JoinedAt     time.Time `json:"joined_at"`
	IsVR         bool      `json:"is_vr"`
}

// MatchSettingsUpdateData is emitted when `g_movement` or `g_gameplay`
// changes mid-match (CvarChange log lines). Either field may be empty;
// only the one that changed is populated.
type MatchSettingsUpdateData struct {
	MatchUUID string `json:"match_uuid"`
	Movement  string `json:"movement,omitempty"`
	Gameplay  string `json:"gameplay,omitempty"`
}

// MatchCrashedData is emitted when a new InitGame arrives while a
// previous match is still open (i.e., no Exit or Shutdown was seen).
// The hub writer marks the old match as exit_reason="crashed".
type MatchCrashedData struct {
	MatchUUID string    `json:"match_uuid"`
	EndedAt   time.Time `json:"ended_at"`
}

// PlayerJoinData is emitted on ClientBegin. MatchUUID is empty when the
// player joins outside a live match (warmup, between matches).
// client_engine / client_version are NOT set here — they arrive later
// via TrinityHandshakeData once the handshake completes.
type PlayerJoinData struct {
	MatchUUID string    `json:"match_uuid,omitempty"`
	GUID      string    `json:"guid"`
	Name      string    `json:"name"`
	CleanName string    `json:"clean_name"`
	Model     string    `json:"model,omitempty"`
	IP        string    `json:"ip,omitempty"`
	IsBot     bool      `json:"is_bot"`
	IsVR      bool      `json:"is_vr"`
	// Skill is the bot's q3 skill level (1-5). Zero for humans.
	// Carried so the hub's presence tracker can render the bot badge
	// tier on the live dashboard.
	Skill    float64   `json:"skill,omitempty"`
	JoinedAt time.Time `json:"joined_at"`
	// ClientNum is the game server's slot for the player (0-31). The
	// hub's presence tracker keys on (serverID, ClientNum) so UDP
	// statusResponse rows — which carry only name, ping, score, and
	// this slot number — can be enriched with identity.
	ClientNum int `json:"client_num"`
}

// PresenceSnapshotData restores hub presence for a player the collector
// already has in state.clients — used after a hub/collector restart
// while a match is in progress. Distinct from PlayerJoinData because
// no one is actually joining: no session creation, no "X joined"
// broadcast, just a state refresh keyed on (ServerID, ClientNum).
type PresenceSnapshotData struct {
	GUID         string  `json:"guid"`
	Name         string  `json:"name"`
	CleanName    string  `json:"clean_name"`
	Model        string  `json:"model,omitempty"`
	IsBot        bool    `json:"is_bot"`
	IsVR         bool    `json:"is_vr"`
	Skill        float64 `json:"skill,omitempty"`
	ClientNum    int     `json:"client_num"`
	Impressives  int     `json:"impressives,omitempty"`
	Excellents   int     `json:"excellents,omitempty"`
	Humiliations int     `json:"humiliations,omitempty"`
	Defends      int     `json:"defends,omitempty"`
	Captures     int     `json:"captures,omitempty"`
	Assists      int     `json:"assists,omitempty"`
}

// PlayerLeaveData is emitted on ClientDisconnect. DurationSeconds is
// computed by the collector from its in-memory JoinedAt; the hub writer
// uses it to synthesize JoinedAt if the matching open session was lost
// (hub restart).
type PlayerLeaveData struct {
	GUID            string    `json:"guid"`
	ClientNum       int       `json:"client_num"`
	LeftAt          time.Time `json:"left_at"`
	DurationSeconds int       `json:"duration_seconds"`
	Reason          string    `json:"reason,omitempty"`
}

// TrinityHandshakeData carries the engine/version portion of a Trinity
// handshake onto the session row. The optional auth portion of the same
// handshake flows separately through the greet RPC.
type TrinityHandshakeData struct {
	GUID          string `json:"guid"`
	ClientEngine  string `json:"client_engine"`
	ClientVersion string `json:"client_version"`
}

// ServerStartupData is emitted when the collector observes its game
// server initializing (log line indicating a fresh sv startup). The hub
// writer closes any sessions still open on this server from before the
// timestamp — a crash-recovery safety net.
type ServerStartupData struct {
	StartedAt time.Time `json:"started_at"`
}

// ServerShutdownData is emitted on a clean ServerShutdown log line.
// The hub writer closes all open sessions on this server at the given
// timestamp.
type ServerShutdownData struct {
	ShutdownAt time.Time `json:"shutdown_at"`
}

// DemoFinalizedData is emitted when trinity-engine logs a "DemoSaved:"
// line for a match — i.e. the .tvd file has been finalized on disk
// and is fetchable from the source's public_url. The hub writer flips
// matches.demo_available so the UI renders a play button. Discard
// events aren't emitted: absence of FactDemoFinalized for a match is
// the same signal.
type DemoFinalizedData struct {
	MatchUUID  string `json:"match_uuid"`
	Frames     int    `json:"frames,omitempty"`
	DurationMS int    `json:"duration_ms,omitempty"`
	Bytes      uint64 `json:"bytes,omitempty"`
}
