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
	Mode       string     `json:"mode,omitempty"`
}

const (
	GameTypeFFA       = "ffa"
	GameTypeTDM       = "tdm"
	GameTypeCTF       = "ctf"
	GameType1v1       = "1v1"
	GameType1FCTF     = "1fctf"
	GameTypeOverload  = "overload"
	GameTypeHarvester = "harvester"
)

// GameTypeFromInt maps a g_gametype value to its name; 5, 6 and 7 are
// the Team Arena game types.
func GameTypeFromInt(gt int) string {
	switch gt {
	case 0:
		return GameTypeFFA
	case 1:
		return GameType1v1
	case 3:
		return GameTypeTDM
	case 4:
		return GameTypeCTF
	case 5:
		return GameType1FCTF
	case 6:
		return GameTypeOverload
	case 7:
		return GameTypeHarvester
	default:
		return "unknown"
	}
}

// MatchPlayerSummary represents a player's participation in a match
type MatchPlayerSummary struct {
	PlayerID          int64    `json:"player_id"`
	Name              string   `json:"name"`
	CleanName         string   `json:"clean_name"`
	Frags             int      `json:"frags"`
	Deaths            int      `json:"deaths"`
	Completed         bool     `json:"completed"`
	IsBot             bool     `json:"is_bot"`
	IsVR              bool     `json:"is_vr"`
	IsVerified        bool     `json:"is_verified"`
	IsAdmin           bool     `json:"is_admin"`
	Skill             *float64 `json:"skill,omitempty"`
	Score             *int     `json:"score,omitempty"`
	Team              *int     `json:"team,omitempty"`
	Model             string   `json:"model,omitempty"`
	Impressives       int      `json:"impressives,omitempty"`
	Excellents        int      `json:"excellents,omitempty"`
	Humiliations      int      `json:"humiliations,omitempty"`
	Defends           int      `json:"defends,omitempty"`
	Victories         int      `json:"victories,omitempty"`
	Captures          int      `json:"captures,omitempty"`
	Assists           int      `json:"assists,omitempty"`
	FlagReturns       int      `json:"flag_returns,omitempty"`
	SkullsDelivered   int      `json:"skulls_delivered,omitempty"`
	ObelisksDestroyed int      `json:"obelisks_destroyed,omitempty"`
}

// MatchSummary is a match with its server and players. Source and
// ServerKey identify the server; the UI shows "<source> / <key>" and
// dims the card when ServerActive is false. DemoAvailable means the
// .tvd was finalized, locally or remotely; populateDemoURLs sets
// DemoURL from it.
type MatchSummary struct {
	ID            int64                `json:"id"`
	UUID          string               `json:"-"`
	DemoURL       string               `json:"demo_url,omitempty"`
	ServerID      int64                `json:"server_id"`
	ServerKey     string               `json:"server_key"`
	ServerActive  bool                 `json:"server_active"`
	Source        string               `json:"source"`
	MapName       string               `json:"map_name"`
	DemoAvailable bool                 `json:"demo_available"`
	IsFeatured    bool                 `json:"is_featured"`
	GameType      string               `json:"game_type"`
	StartedAt     time.Time            `json:"started_at"`
	EndedAt       *time.Time           `json:"ended_at,omitempty"`
	ExitReason    string               `json:"exit_reason,omitempty"`
	Players       []MatchPlayerSummary `json:"players"`
	RedScore      *int                 `json:"red_score,omitempty"`
	BlueScore     *int                 `json:"blue_score,omitempty"`
	Mode          string               `json:"mode,omitempty"`
}
