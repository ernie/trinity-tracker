package domain

import "time"

// Server represents a Quake 3 server being monitored
type Server struct {
	ID                int64      `json:"id"`
	Name              string     `json:"name"`
	Address           string     `json:"address"`
	LogPath           string     `json:"log_path,omitempty"`
	LastMatchUUID     *string    `json:"last_match_uuid,omitempty"`
	LastMatchEndedAt  *time.Time `json:"last_match_ended_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

// ServerStatus represents the current state of a server from UDP query
type ServerStatus struct {
	ServerID        int64             `json:"server_id"`
	Name            string            `json:"name"`
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
	ServerVars      map[string]string `json:"server_vars,omitempty"`
	TeamScores      *TeamScores       `json:"team_scores,omitempty"`
	FlagStatus      *FlagStatus       `json:"flag_status,omitempty"`
	MatchState      string            `json:"match_state,omitempty"`       // "waiting", "warmup", "active", "intermission"
	WarmupRemaining int               `json:"warmup_remaining,omitempty"` // milliseconds remaining in warmup
}

// TeamScores represents team scores for team game modes
type TeamScores struct {
	RedScore  int `json:"red"`
	BlueScore int `json:"blue"`
}

// FlagStatus represents CTF flag states
// Status values: 0=at base, 1=taken, 2=dropped
// Carrier values: client_num of carrier, or -1 if not carried
type FlagStatus struct {
	Red         int `json:"red"`
	RedCarrier  int `json:"red_carrier"`
	Blue        int `json:"blue"`
	BlueCarrier int `json:"blue_carrier"`
}

// PlayerStatus represents a player's current state on a server
type PlayerStatus struct {
	ClientNum    int       `json:"client_num"`
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
	Model        string    `json:"model,omitempty"`        // player model (e.g., "sarge/krusade")
}
