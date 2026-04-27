package domain

import "time"

// Event types for WebSocket notifications
const (
	EventPlayerJoin    = "player_join"
	EventPlayerLeave   = "player_leave"
	EventServerUpdate  = "server_update"
	EventMatchStart    = "match_start"
	EventMatchEnd      = "match_end"
	EventFrag          = "frag"
	EventFlagCapture   = "flag_capture"
	EventFlagTaken     = "flag_taken"
	EventFlagReturn    = "flag_return"
	EventFlagDrop      = "flag_drop"
	EventObeliskDestroy = "obelisk_destroy"
	EventSkullScore    = "skull_score"
	EventTeamChange    = "team_change"
	EventSay           = "say"
	EventSayTeam       = "say_team"
	EventTell          = "tell"
	EventSayRcon       = "say_rcon"
	EventAward         = "award"
)

// Event represents a real-time event for WebSocket broadcast
type Event struct {
	Type      string      `json:"event"`
	ServerID  int64       `json:"server_id"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
}

// PlayerJoinEvent is sent when a player connects
type PlayerJoinEvent struct {
	Player   PlayerStatus `json:"player"`
	PlayerID *int64       `json:"player_id,omitempty"`
}

// PlayerLeaveEvent is sent when a player disconnects
type PlayerLeaveEvent struct {
	PlayerName string `json:"player_name"`
	GUID       string `json:"guid,omitempty"`
	PlayerID   *int64 `json:"player_id,omitempty"`
}

// MatchStartEvent is sent when a new map starts
type MatchStartEvent struct {
	Map      string `json:"map"`
	GameType string `json:"game_type"`
}

// MatchEndEvent is sent when a match ends
type MatchEndEvent struct {
	ExitReason string `json:"exit_reason"`
}

// FragEvent is sent when a frag occurs
type FragEvent struct {
	Fragger         string `json:"fragger"`
	Victim          string `json:"victim"`
	Weapon          string `json:"weapon"`
	FraggerGUID     string `json:"fragger_guid,omitempty"`
	VictimGUID      string `json:"victim_guid,omitempty"`
	FraggerPlayerID *int64 `json:"fragger_player_id,omitempty"`
	VictimPlayerID  *int64 `json:"victim_player_id,omitempty"`
}

// FlagCaptureEvent is sent when a flag is captured (CTF)
type FlagCaptureEvent struct {
	ClientNum  int    `json:"client_num"`
	PlayerName string `json:"player_name"`
	Team       int    `json:"team"` // team that scored (captured enemy flag)
	GUID       string `json:"guid,omitempty"`
	PlayerID   *int64 `json:"player_id,omitempty"`
}

// FlagTakenEvent is sent when a flag is picked up
type FlagTakenEvent struct {
	ClientNum  int    `json:"client_num"`
	PlayerName string `json:"player_name"`
	Team       int    `json:"team"` // team of the flag that was taken
	GUID       string `json:"guid,omitempty"`
	PlayerID   *int64 `json:"player_id,omitempty"`
}

// FlagReturnEvent is sent when a flag is returned
type FlagReturnEvent struct {
	ClientNum  int    `json:"client_num"` // -1 for auto-return
	PlayerName string `json:"player_name"` // may be empty for auto-return
	Team       int    `json:"team"`        // team of the flag that was returned
	GUID       string `json:"guid,omitempty"`
	PlayerID   *int64 `json:"player_id,omitempty"`
}

// FlagDropEvent is sent when a flag is dropped
type FlagDropEvent struct {
	ClientNum  int    `json:"client_num"`
	PlayerName string `json:"player_name"`
	Team       int    `json:"team"` // team of the flag that was dropped
	GUID       string `json:"guid,omitempty"`
	PlayerID   *int64 `json:"player_id,omitempty"`
}

// ObeliskDestroyEvent is sent when an obelisk is destroyed (Overload mode)
type ObeliskDestroyEvent struct {
	AttackerName string `json:"attacker_name"`
	Team         int    `json:"team"` // team whose obelisk was destroyed
	GUID         string `json:"guid,omitempty"`
	PlayerID     *int64 `json:"player_id,omitempty"`
}

// SkullScoreEvent is sent when skulls are scored (Harvester mode)
type SkullScoreEvent struct {
	PlayerName string `json:"player_name"`
	Team       int    `json:"team"`
	Skulls     int    `json:"skulls"`
	GUID       string `json:"guid,omitempty"`
	PlayerID   *int64 `json:"player_id,omitempty"`
}

// TeamChangeEvent is sent when a player changes teams
type TeamChangeEvent struct {
	PlayerName string `json:"player_name"`
	OldTeam    int    `json:"old_team"`
	NewTeam    int    `json:"new_team"`
	GUID       string `json:"guid,omitempty"`
	PlayerID   *int64 `json:"player_id,omitempty"`
}

// SayEvent is sent when a player sends a global chat message
type SayEvent struct {
	ClientNum  int    `json:"client_num"`
	PlayerName string `json:"player_name"`
	Message    string `json:"message"`
	GUID       string `json:"guid,omitempty"`
	PlayerID   *int64 `json:"player_id,omitempty"`
}

// SayTeamEvent is sent when a player sends a team chat message
type SayTeamEvent struct {
	ClientNum  int    `json:"client_num"`
	PlayerName string `json:"player_name"`
	Message    string `json:"message"`
	GUID       string `json:"guid,omitempty"`
	PlayerID   *int64 `json:"player_id,omitempty"`
}

// TellEvent is sent when a player sends a private message
type TellEvent struct {
	FromClientNum  int    `json:"from_client_num"`
	ToClientNum    int    `json:"to_client_num"`
	FromName       string `json:"from_name"`
	ToName         string `json:"to_name"`
	Message        string `json:"message"`
	FromGUID       string `json:"from_guid,omitempty"`
	ToGUID         string `json:"to_guid,omitempty"`
	FromPlayerID   *int64 `json:"from_player_id,omitempty"`
	ToPlayerID     *int64 `json:"to_player_id,omitempty"`
}

// SayRconEvent is sent when an RCON message is broadcast
type SayRconEvent struct {
	Message string `json:"message"`
}

// AwardEvent is sent when a player earns an award (impressive, excellent, humiliation, defend, assist)
type AwardEvent struct {
	ClientNum      int    `json:"client_num"`
	PlayerName     string `json:"player_name"`
	AwardType      string `json:"award_type"` // impressive, excellent, humiliation, defend, assist
	Team           int    `json:"team,omitempty"`             // player's team (1=Red, 2=Blue)
	GUID           string `json:"guid,omitempty"`
	PlayerID       *int64 `json:"player_id,omitempty"`
	VictimName     string `json:"victim_name,omitempty"`      // for humiliation awards
	VictimGUID     string `json:"victim_guid,omitempty"`      // for humiliation awards
	VictimPlayerID *int64 `json:"victim_player_id,omitempty"` // for humiliation awards
}
