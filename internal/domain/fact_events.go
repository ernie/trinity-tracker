package domain

import "time"

// Fact events flow from the collector to the hub writer and drive the
// database writes. Payloads identify players and matches by GUID and
// match UUID, since database row IDs differ between collector and hub.

const (
	FactMatchStart          = "match_start"
	FactMatchEnd            = "match_end"
	FactMatchSettingsUpdate = "match_settings_update"
	FactMatchCrashed        = "match_crashed"
	FactPlayerJoin          = "player_join"
	FactPlayerLeave         = "player_leave"
	FactPresenceSnapshot    = "presence_snapshot"
	FactTrinityHandshake    = "trinity_handshake"
	FactPlayerVR            = "player_vr"
	FactServerStartup       = "server_startup"
	FactServerShutdown      = "server_shutdown"
	FactDemoFinalized       = "demo_finalized"
	// FactRconExec is one accepted /rcon command the engine logged
	// (SVC_RemoteCommand in sv_main.c). It writes a source_audit row with
	// action "rcon.exec", the same action web RCON writes, so both share
	// one audit view.
	FactRconExec = "rcon_exec"
	// FactRconDenied is one rejected /rcon attempt (wrong or unset
	// password). It writes a source_audit row with action "rcon.denied".
	FactRconDenied = "rcon_denied"
)

// FactEvent carries a payload from the collector to the hub writer in
// process. The NATS Envelope adds source, seq and schema version.
type FactEvent struct {
	Type      string      `json:"type"`
	ServerID  int64       `json:"server_id"`
	Timestamp time.Time   `json:"ts"`
	Data      interface{} `json:"data"`
}

// MatchStartData is emitted on WarmupEnd. The collector assigns
// MatchUUID, which keys the match's later events.
//
// HandshakeRequired reflects the server's g_trinityHandshake cvar. The
// collector publishes match_start only when it's set, and the hub
// checks again, so stats come only from servers that admit Trinity
// clients alone. The gate is on the handshake itself; whether it
// carries a valid login doesn't matter here.
type MatchStartData struct {
	MatchUUID         string    `json:"match_uuid"`
	MapName           string    `json:"map"`
	GameType          string    `json:"gametype"`
	Mode              string    `json:"mode,omitempty"`
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
	GUID              string    `json:"guid"`
	ClientID          int       `json:"client_id"`
	Name              string    `json:"name"`
	CleanName         string    `json:"clean_name"`
	Frags             int       `json:"frags"`
	Deaths            int       `json:"deaths"`
	Completed         bool      `json:"completed"`
	Score             *int      `json:"score,omitempty"`
	Team              *int      `json:"team,omitempty"`
	Model             string    `json:"model,omitempty"`
	Skill             float64   `json:"skill,omitempty"`
	Victory           bool      `json:"victory"`
	Captures          int       `json:"captures"`
	FlagReturns       int       `json:"flag_returns"`
	Assists           int       `json:"assists"`
	Impressives       int       `json:"impressives"`
	Excellents        int       `json:"excellents"`
	Humiliations      int       `json:"humiliations"`
	Defends           int       `json:"defends"`
	SkullsDelivered   int       `json:"skulls_delivered"`   // Harvester (skulls deposited at enemy obelisk)
	ObelisksDestroyed int       `json:"obelisks_destroyed"` // Overload
	IsBot             bool      `json:"is_bot"`
	JoinedLate        bool      `json:"joined_late"`
	JoinedAt          time.Time `json:"joined_at"`
	IsVR              bool      `json:"is_vr"`
}

// MatchSettingsUpdateData is emitted when g_mode changes mid-match.
type MatchSettingsUpdateData struct {
	MatchUUID string `json:"match_uuid"`
	Mode      string `json:"mode,omitempty"`
}

// MatchCrashedData is emitted when a new InitGame arrives while the
// previous match is still open, with no Exit or Shutdown seen. The hub
// writer sets the old match's exit_reason to "crashed".
type MatchCrashedData struct {
	MatchUUID string    `json:"match_uuid"`
	EndedAt   time.Time `json:"ended_at"`
}

// PlayerJoinData is emitted on ClientBegin. MatchUUID is empty when the
// player joins outside a live match (warmup, between matches). The
// client engine and version arrive later in TrinityHandshakeData.
type PlayerJoinData struct {
	MatchUUID string `json:"match_uuid,omitempty"`
	GUID      string `json:"guid"`
	Name      string `json:"name"`
	CleanName string `json:"clean_name"`
	Model     string `json:"model,omitempty"`
	IP        string `json:"ip,omitempty"`
	IsBot     bool   `json:"is_bot"`
	IsVR      bool   `json:"is_vr"`
	// Skill is a bot's skill level, 1-5, for the live card's bot badge;
	// zero for humans.
	Skill    float64   `json:"skill,omitempty"`
	JoinedAt time.Time `json:"joined_at"`
	// ClientNum is the player's slot (0-31). Presence keys on
	// (serverID, ClientNum) so UDP status rows, which carry only name,
	// ping, score and slot, can be matched to an identity.
	ClientNum int `json:"client_num"`
}

// PresenceSnapshotData restores hub presence for a player already in
// the collector's state.clients, after a hub or collector restart
// mid-match. It only refreshes state keyed on (ServerID, ClientNum);
// no session starts and no join is announced.
type PresenceSnapshotData struct {
	GUID            string  `json:"guid"`
	Name            string  `json:"name"`
	CleanName       string  `json:"clean_name"`
	Model           string  `json:"model,omitempty"`
	IsBot           bool    `json:"is_bot"`
	IsVR            bool    `json:"is_vr"`
	Skill           float64 `json:"skill,omitempty"`
	ClientNum       int     `json:"client_num"`
	Impressives     int     `json:"impressives,omitempty"`
	Excellents      int     `json:"excellents,omitempty"`
	Humiliations    int     `json:"humiliations,omitempty"`
	Defends         int     `json:"defends,omitempty"`
	Captures        int     `json:"captures,omitempty"`
	Assists         int     `json:"assists,omitempty"`
	SkullsDelivered int     `json:"skulls_delivered,omitempty"`
}

// PlayerLeaveData is emitted on ClientDisconnect. The collector computes
// DurationSeconds from its in-memory JoinedAt; the hub writer uses it to
// rebuild JoinedAt when a hub restart lost the open session.
type PlayerLeaveData struct {
	GUID            string    `json:"guid"`
	ClientNum       int       `json:"client_num"`
	LeftAt          time.Time `json:"left_at"`
	DurationSeconds int       `json:"duration_seconds"`
	Reason          string    `json:"reason,omitempty"`
}

// TrinityHandshakeData carries a Trinity handshake's engine, version
// and platform onto the session row. The handshake's optional login
// goes through the greet RPC.
type TrinityHandshakeData struct {
	GUID          string `json:"guid"`
	ClientEngine  string `json:"client_engine"`
	ClientVersion string `json:"client_version"`
	Platform      string `json:"platform"`
}

// PlayerVRData reports a joined player switching into VR.
type PlayerVRData struct {
	GUID string `json:"guid"`
}

// ServerStartupData is emitted when the game server's log shows a fresh
// startup. The hub writer closes any sessions on this server still open
// from before it, which cleans up after a crash.
type ServerStartupData struct {
	StartedAt time.Time `json:"started_at"`
}

// ServerShutdownData is emitted on a clean ServerShutdown log line.
// The hub writer closes all open sessions on this server at the given
// timestamp.
type ServerShutdownData struct {
	ShutdownAt time.Time `json:"shutdown_at"`
}

// RconExecData carries one in-game /rcon command for the hub's audit
// log. GUID is empty when the engine couldn't match the sender's
// address to a connected client (external rcon, NAT, a separate
// console socket). The hub writer writes a row only when the GUID
// resolves to a known user; the rest is the collector's own traffic
// (welcome messages, web RCON looping back through the local socket)
// or rcon nobody can attribute.
type RconExecData struct {
	ClientAddr string `json:"client_addr"`
	GUID       string `json:"guid,omitempty"`
	Command    string `json:"command"`
}

// RconDeniedData is emitted when the engine rejects an rcon attempt
// (wrong or unset password), for spotting brute-force attempts. It
// carries only the sender's address, keeping passwords out of the
// audit log.
type RconDeniedData struct {
	ClientAddr string `json:"client_addr"`
}

// DemoFinalizedData is emitted when the engine logs "DemoSaved:" for a
// match: the .tvd is complete on disk and served from the source's
// public_url. The hub writer sets matches.demo_available, which shows
// the play button. A discarded demo sends nothing.
type DemoFinalizedData struct {
	MatchUUID  string `json:"match_uuid"`
	Frames     int    `json:"frames,omitempty"`
	DurationMS int    `json:"duration_ms,omitempty"`
	Bytes      uint64 `json:"bytes,omitempty"`
}
