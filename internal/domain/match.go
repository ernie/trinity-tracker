package domain

import "time"

// Match represents a game session on a server
type Match struct {
	ID         int64      `json:"id"`
	UUID       string     `json:"uuid,omitempty"` // unique match identifier from game
	ServerID   int64      `json:"server_id"`
	MapName    string     `json:"map_name"`
	GameType   string     `json:"game_type"`
	StartedAt  time.Time  `json:"started_at"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
	ExitReason string     `json:"exit_reason,omitempty"`
	RedScore   *int       `json:"red_score,omitempty"`
	BlueScore  *int       `json:"blue_score,omitempty"`
}

// GameType constants
const (
	GameTypeFFA  = "ffa"
	GameTypeTDM  = "tdm"
	GameTypeCTF  = "ctf"
	GameType1v1  = "1v1"
)

// GameTypeFromInt converts Q3's numeric gametype to string
func GameTypeFromInt(gt int) string {
	switch gt {
	case 0:
		return GameTypeFFA
	case 3:
		return GameTypeTDM
	case 4:
		return GameTypeCTF
	case 1:
		return GameType1v1
	default:
		return "unknown"
	}
}

// MatchPlayerSummary represents a player's participation in a match
type MatchPlayerSummary struct {
	PlayerID     int64    `json:"player_id"`
	Name         string   `json:"name"`
	CleanName    string   `json:"clean_name"`
	Kills        int      `json:"kills"`
	Deaths       int      `json:"deaths"`
	Completed    bool     `json:"completed"`
	IsBot        bool     `json:"is_bot"`
	Skill        *float64 `json:"skill,omitempty"`
	Score        *int     `json:"score,omitempty"`
	Team         *int     `json:"team,omitempty"`
	Model        string   `json:"model,omitempty"`
	Impressives  int      `json:"impressives,omitempty"`
	Excellents   int      `json:"excellents,omitempty"`
	Humiliations int      `json:"humiliations,omitempty"`
	Defends      int      `json:"defends,omitempty"`
	Captures     int      `json:"captures,omitempty"`
	Assists      int      `json:"assists,omitempty"`
}

// MatchSummary represents a match with server and player info
type MatchSummary struct {
	ID         int64                `json:"id"`
	ServerID   int64                `json:"server_id"`
	ServerName string               `json:"server_name"`
	MapName    string               `json:"map_name"`
	GameType   string               `json:"game_type"`
	StartedAt  time.Time            `json:"started_at"`
	EndedAt    *time.Time           `json:"ended_at,omitempty"`
	ExitReason string               `json:"exit_reason,omitempty"`
	Players    []MatchPlayerSummary `json:"players"`
	RedScore   *int                 `json:"red_score,omitempty"`
	BlueScore  *int                 `json:"blue_score,omitempty"`
}
