package domain

import "time"

// Fact events flow from the collector to the hub writer and drive
// authoritative database writes. They are distinct from the live events
// above (which are ephemeral WebSocket broadcasts).
//
// In standalone mode these travel through an in-process channel; in
// distributed mode they are serialized into NATS envelopes. The payload
// shapes are identical in both transports so the collector and hub code
// does not change across M1 and M2.
//
// Wire identity rule: payloads carry GUIDs and match UUIDs, never DB row
// IDs. The hub writer owns the translation from wire identity to DB ID.

const (
	FactMatchStart           = "match_start"
	FactMatchEnd             = "match_end"
	FactMatchSettingsUpdate  = "match_settings_update"
	FactMatchCrashed         = "match_crashed"
	FactPlayerJoin           = "player_join"
	FactPlayerLeave          = "player_leave"
	FactTrinityHandshake     = "trinity_handshake"
	FactServerStartup        = "server_startup"
	FactServerShutdown       = "server_shutdown"
)

// FactEvent is the in-process envelope carrying a payload from the
// collector to the hub writer. The NATS wire envelope (see distributed
// tracking design spec) wraps this shape with source/seq/schema_version
// fields; in M1 we carry just what the writer needs.
type FactEvent struct {
	Type      string      `json:"type"`
	ServerID  int64       `json:"server_id"`
	Timestamp time.Time   `json:"ts"`
	Data      interface{} `json:"data"`
}

// MatchStartData is emitted on WarmupEnd when gameplay begins. The
// collector assigns MatchUUID (same one that lands in the log); the hub
// writer inserts a `matches` row keyed on that UUID.
type MatchStartData struct {
	MatchUUID string    `json:"match_uuid"`
	MapName   string    `json:"map"`
	GameType  string    `json:"gametype"`
	Movement  string    `json:"movement,omitempty"`
	Gameplay  string    `json:"gameplay,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

// MatchEndData is emitted on match shutdown (intermission or abrupt).
// Players carries the final stats for every participant (current and
// previous stints) — the hub writer flushes them all in one pass.
type MatchEndData struct {
	MatchUUID  string             `json:"match_uuid"`
	EndedAt    time.Time          `json:"ended_at"`
	ExitReason string             `json:"exit_reason,omitempty"`
	RedScore   *int               `json:"red_score,omitempty"`
	BlueScore  *int               `json:"blue_score,omitempty"`
	Players    []MatchEndPlayer   `json:"players"`
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
	JoinedAt  time.Time `json:"joined_at"`
}

// PlayerLeaveData is emitted on ClientDisconnect. DurationSeconds is
// computed by the collector from its in-memory JoinedAt; the hub writer
// uses it to synthesize JoinedAt if the matching open session was lost
// (hub restart).
type PlayerLeaveData struct {
	GUID            string    `json:"guid"`
	LeftAt          time.Time `json:"left_at"`
	DurationSeconds int       `json:"duration_seconds"`
	Reason          string    `json:"reason,omitempty"`
}

// TrinityHandshakeData is emitted when the Trinity handshake log line
// is parsed. It enriches the open session row with engine/mod version.
// Auth verification is handled out-of-band via the greet RPC, not here.
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
