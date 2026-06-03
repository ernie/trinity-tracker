package domain

import (
	"regexp"
	"time"
)

// Player represents a logical person (can have multiple GUIDs)
type Player struct {
	ID                   int64     `json:"id"`
	Name                 string    `json:"name"`
	CleanName            string    `json:"clean_name"`
	FirstSeen            time.Time `json:"first_seen"`
	LastSeen             time.Time `json:"last_seen"`
	TotalPlaytimeSeconds int64     `json:"total_playtime_seconds"`
	IsBot                bool      `json:"is_bot"`
	IsVR                 bool      `json:"is_vr"`
	Model                string    `json:"model,omitempty"` // most recent model used
	Skill                float64   `json:"skill,omitempty"` // bot skill level (1-5), 0 if human
	IsVerified           bool      `json:"is_verified"`
	IsAdmin              bool      `json:"is_admin"`
	// FeaturedHonor is the linked user's chosen honor key (e.g. "victories",
	// "captures") for the profile hero card. nil = no linked user, or linked
	// user hasn't chosen — the frontend falls back to 'victories'.
	FeaturedHonor *string      `json:"featured_honor,omitempty"`
	GUIDs         []PlayerGUID `json:"guids,omitempty"` // populated when fetching with details
}

// PlayerGUID represents a single GUID belonging to a player
type PlayerGUID struct {
	ID        int64     `json:"id"`
	PlayerID  int64     `json:"player_id"`
	GUID      string    `json:"guid"`
	Name      string    `json:"name"`
	CleanName string    `json:"clean_name"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	IsBot     bool      `json:"is_bot"`
	IsVR      bool      `json:"is_vr"`
}

// Session represents a player's time on a server (linked to a GUID)
type Session struct {
	ID              int64      `json:"id"`
	PlayerGUIDID    int64      `json:"player_guid_id"`
	ServerID        int64      `json:"server_id"`
	JoinedAt        time.Time  `json:"joined_at"`
	LeftAt          *time.Time `json:"left_at,omitempty"`
	DurationSeconds int64      `json:"duration_seconds,omitempty"`
	IPAddress       string     `json:"ip_address,omitempty"`
	ClientEngine    string     `json:"client_engine,omitempty"`
	ClientVersion   string     `json:"client_version,omitempty"`
}

// PlayerSession represents a session for display (includes server name)
type PlayerSession struct {
	ID              int64      `json:"id"`
	ServerID        int64      `json:"server_id"`
	ServerKey       string     `json:"server_key"`
	ServerSource    string     `json:"server_source"`
	JoinedAt        time.Time  `json:"joined_at"`
	LeftAt          *time.Time `json:"left_at,omitempty"`
	DurationSeconds int64      `json:"duration_seconds,omitempty"`
	IPAddress       string     `json:"ip_address,omitempty"`
	ClientEngine    string     `json:"client_engine,omitempty"`
	ClientVersion   string     `json:"client_version,omitempty"`
}

// AdminSession is a session row for the admin sessions view, with player identity included.
type AdminSession struct {
	ID              int64      `json:"id"`
	ServerID        int64      `json:"server_id"`
	ServerKey       string     `json:"server_key"`
	ServerSource    string     `json:"server_source"`
	PlayerID        int64      `json:"player_id"`
	PlayerName      string     `json:"player_name"`
	PlayerCleanName string     `json:"player_clean_name"`
	JoinedAt        time.Time  `json:"joined_at"`
	LeftAt          *time.Time `json:"left_at,omitempty"`
	DurationSeconds int64      `json:"duration_seconds,omitempty"`
	IPAddress       string     `json:"ip_address,omitempty"`
	ClientEngine    string     `json:"client_engine,omitempty"`
	ClientVersion   string     `json:"client_version,omitempty"`
}

// PlayerStats holds aggregated stats for a player (for leaderboards)
type PlayerStats struct {
	Player        Player  `json:"player"`
	TotalFrags    int64   `json:"total_frags"`
	TotalDeaths   int64   `json:"total_deaths"`
	TotalMatches  int64   `json:"total_matches"`
	KDRatio       float64 `json:"kd_ratio"`
	WinCount      int64   `json:"win_count"`
	PlaytimeHours float64 `json:"playtime_hours"`
}

// LeaderboardEntry represents a player's position on a leaderboard
type LeaderboardEntry struct {
	Rank               int     `json:"rank"`
	Player             Player  `json:"player"`
	TotalFrags         int64   `json:"total_frags"`
	TotalDeaths        int64   `json:"total_deaths"`
	TotalMatches       int64   `json:"total_matches"`
	CompletedMatches   int64   `json:"completed_matches"`
	UncompletedMatches int64   `json:"uncompleted_matches"`
	KDRatio            float64 `json:"kd_ratio"`
	Captures           int64   `json:"captures"`
	FlagReturns        int64   `json:"flag_returns"`
	Assists            int64   `json:"assists"`
	Impressives        int64   `json:"impressives"`
	Excellents         int64   `json:"excellents"`
	Humiliations       int64   `json:"humiliations"`
	Defends            int64   `json:"defends"`
	SkullsDelivered    int64   `json:"skulls_delivered"`
	ObelisksDestroyed  int64   `json:"obelisks_destroyed"`
	Victories          int64   `json:"victories"`
}

// LeaderboardResponse is the API response for leaderboard data
type LeaderboardResponse struct {
	Category    string             `json:"category"`
	Period      string             `json:"period"`
	PeriodStart *time.Time         `json:"period_start,omitempty"`
	PeriodEnd   *time.Time         `json:"period_end,omitempty"`
	Entries     []LeaderboardEntry `json:"entries"`
}

// PlayerName represents a historical name used by a player GUID
type PlayerName struct {
	Name      string    `json:"name"`       // original color-coded name
	CleanName string    `json:"clean_name"` // stripped name
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// AggregatedStats holds time-filtered stats for a player
type AggregatedStats struct {
	Matches            int64   `json:"matches"`
	CompletedMatches   int64   `json:"completed_matches"`
	UncompletedMatches int64   `json:"uncompleted_matches"`
	Frags              int64   `json:"frags"`
	Deaths             int64   `json:"deaths"`
	KDRatio            float64 `json:"kd_ratio"`
	Captures           int64   `json:"captures"`
	FlagReturns        int64   `json:"flag_returns"`
	Assists            int64   `json:"assists"`
	Impressives        int64   `json:"impressives"`
	Excellents         int64   `json:"excellents"`
	Humiliations       int64   `json:"humiliations"`
	Defends            int64   `json:"defends"`
	SkullsDelivered    int64   `json:"skulls_delivered"`
	ObelisksDestroyed  int64   `json:"obelisks_destroyed"`
	Victories          int64   `json:"victories"`
}

// PlayerStatsResponse is the API response for player stats with time filtering
type PlayerStatsResponse struct {
	Player      Player          `json:"player"`
	Period      string          `json:"period"`
	PeriodStart *time.Time      `json:"period_start,omitempty"`
	PeriodEnd   *time.Time      `json:"period_end,omitempty"`
	Stats       AggregatedStats `json:"stats"`
	Names       []PlayerName    `json:"names"`
}

// PlayerProfile is used for search results and basic player info
type PlayerProfile struct {
	ID                   int64  `json:"id"`
	Name                 string `json:"name"`
	CleanName            string `json:"clean_name"`
	FirstSeen            string `json:"first_seen"`
	LastSeen             string `json:"last_seen"`
	TotalPlaytimeSeconds int64  `json:"total_playtime_seconds"`
	IsVR                 bool   `json:"is_vr"`
}

// q3ColorCodeRegex matches Quake 3 color codes like ^1, ^2, etc.
var q3ColorCodeRegex = regexp.MustCompile(`\^[0-9]`)

// CleanQ3Name removes Quake 3 color codes from a player name
func CleanQ3Name(name string) string {
	return q3ColorCodeRegex.ReplaceAllString(name, "")
}

// CleanQ3DisplayName combines the engine's Info_SetValueForKey restrictions
// (\, ", ; stripped) with the QVM's BG_CleanName algorithm: strips leading
// whitespace, drops black color codes (^0, ^8), removes non-printable chars,
// collapses runs of more than 2 spaces, and caps output at 31 bytes.
// Returns empty string if the result has no visible (non-color-code) characters.
func CleanQ3DisplayName(raw string) string {
	const maxBytes = 31

	var out []byte
	colorlessLen := 0
	spaces := 0
	i := 0

	for i < len(raw) {
		ch := raw[i]

		if len(out) == 0 && ch <= ' ' {
			i++
			continue
		}

		if ch == '^' && i+1 < len(raw) {
			next := raw[i+1]
			if next >= '0' && next <= '9' {
				colorIdx := (next - '0') & 7
				if colorIdx == 0 {
					i += 2
					continue
				}
				if len(out) > maxBytes-2 {
					break
				}
				out = append(out, ch, next)
				i += 2
				continue
			}
		}

		if ch < ' ' || ch > 126 || ch == '\\' || ch == '"' || ch == ';' {
			i++
			continue
		}

		if ch == ' ' {
			spaces++
			if spaces > 2 {
				i++
				continue
			}
		} else {
			spaces = 0
		}

		if len(out) > maxBytes-1 {
			break
		}

		out = append(out, ch)
		colorlessLen++
		i++
	}

	if colorlessLen == 0 {
		return ""
	}
	return string(out)
}

// vrLeadingRe matches a leading "[VR]" tag, optionally preceded by
// Q3 color runs (e.g. "^7[VR] alice") and followed by whitespace.
var vrLeadingRe = regexp.MustCompile(`(?i)^(?:\^[0-9])*\[VR\]\s*`)

// vrTrailingRe matches a trailing "[VR]" tag with optional leading
// whitespace and trailing color runs ("alice [VR]^7").
var vrTrailingRe = regexp.MustCompile(`(?i)\s*\[VR\](?:\^[0-9])*$`)

// StripVRTag removes the "[VR]" decoration the Trinity VR client
// adds to a player's name, at either the start or end.
func StripVRTag(name string) string {
	if loc := vrLeadingRe.FindStringIndex(name); loc != nil {
		return name[loc[1]:]
	}
	if loc := vrTrailingRe.FindStringIndex(name); loc != nil {
		return name[:loc[0]]
	}
	return name
}
