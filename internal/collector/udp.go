package collector

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

const (
	q3Header    = "\xff\xff\xff\xff"
	getStatus   = q3Header + "getstatus\n"
	rconPrefix  = q3Header + "rcon "
	printPrefix = q3Header + "print\n"
	timeout     = 2 * time.Second
	rconTimeout = 3 * time.Second
	maxResponse = 65535
)

// Q3Client queries Quake 3 servers via UDP
type Q3Client struct{}

// NewQ3Client creates a new Q3 UDP client
func NewQ3Client() *Q3Client {
	return &Q3Client{}
}

// QueryStatus queries a Q3 server and returns its status
func (c *Q3Client) QueryStatus(address string) (*domain.ServerStatus, error) {
	conn, err := net.DialTimeout("udp", address, timeout)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", address, err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(timeout))

	// Send getstatus request
	if _, err := conn.Write([]byte(getStatus)); err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}

	// Read response
	buf := make([]byte, maxResponse)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	return parseStatusResponse(address, buf[:n])
}

// RconCommand sends an RCON command to a Q3 server and returns the response
func (c *Q3Client) RconCommand(address, password, command string) (string, error) {
	conn, err := net.DialTimeout("udp", address, rconTimeout)
	if err != nil {
		return "", fmt.Errorf("connecting to %s: %w", address, err)
	}
	defer conn.Close()

	// Format: \xff\xff\xff\xffrcon <password> <command>
	request := fmt.Sprintf("%s%s %s", rconPrefix, password, command)
	if _, err := conn.Write([]byte(request)); err != nil {
		return "", fmt.Errorf("sending rcon command: %w", err)
	}

	// Read response (may come in multiple packets for long output)
	var response strings.Builder
	buf := make([]byte, maxResponse)

	for {
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, err := conn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break // No more data
			}
			if response.Len() > 0 {
				break // Got some data, treat timeout as end
			}
			return "", fmt.Errorf("reading response: %w", err)
		}

		data := string(buf[:n])
		if strings.HasPrefix(data, printPrefix) {
			response.WriteString(strings.TrimPrefix(data, printPrefix))
		} else if strings.HasPrefix(data, q3Header+"print\n") {
			// Handle slight variations in response format
			response.WriteString(strings.TrimPrefix(data, q3Header+"print\n"))
		}
	}

	return response.String(), nil
}

// parseStatusResponse parses the raw response from a Q3 server
func parseStatusResponse(address string, data []byte) (*domain.ServerStatus, error) {
	response := string(data)

	// Response format: \xff\xff\xff\xffstatusResponse\n<vars>\n<player1>\n<player2>...
	if !strings.HasPrefix(response, q3Header+"statusResponse\n") {
		return nil, fmt.Errorf("invalid response prefix")
	}

	// Remove header
	response = strings.TrimPrefix(response, q3Header+"statusResponse\n")

	lines := strings.Split(response, "\n")
	if len(lines) < 1 {
		return nil, fmt.Errorf("no data in response")
	}

	status := &domain.ServerStatus{
		Address:     address,
		Online:      true,
		LastUpdated: time.Now().UTC(),
		ServerVars:  make(map[string]string),
	}

	// Parse server vars (first line, backslash-separated key/value pairs)
	vars := parseVars(lines[0])
	status.ServerVars = vars

	// Extract common vars
	status.Map = vars["mapname"]
	if gt, err := strconv.Atoi(vars["g_gametype"]); err == nil {
		status.GameType = domain.GameTypeFromInt(gt)
	}
	if mc, err := strconv.Atoi(vars["sv_maxclients"]); err == nil {
		status.MaxClients = mc
	}
	if name := vars["sv_hostname"]; name != "" {
		status.Key = domain.CleanQ3Name(name)
	}

	// Extract team scores for team game modes (CTF, TDM)
	if isTeamGameType(status.GameType) {
		redScore, redOk := parseIntVar(vars, "g_redscore", "score_red")
		blueScore, blueOk := parseIntVar(vars, "g_bluescore", "score_blue")
		if redOk || blueOk {
			status.TeamScores = &domain.TeamScores{
				RedScore:  redScore,
				BlueScore: blueScore,
			}
		}
	}

	var objTail map[int]int
	if objStatus := vars["g_objstatus"]; objStatus != "" {
		status.ObjStatus, objTail = parseObjStatus(objStatus, status.GameType)
	}

	if hp, ok := parseIntVar(vars, "g_obeliskhealth"); ok {
		status.ObeliskHealthMax = hp
	}

	// Extract match state (from enhanced game logging)
	if matchState := vars["g_matchstate"]; matchState != "" {
		status.MatchState = matchState
	}

	// Calculate game time and warmup remaining from level time cvars
	if levelTime, ok := parseIntVar(vars, "g_leveltime"); ok {
		if levelStartTime, ok := parseIntVar(vars, "g_levelstarttime"); ok {
			status.GameTimeMs = levelTime - levelStartTime
		}
		// Calculate warmup remaining from absolute warmup end time
		if warmupEndTime, ok := parseIntVar(vars, "g_warmupendtime"); ok && warmupEndTime > 0 {
			status.WarmupRemaining = warmupEndTime - levelTime
		}
	}

	// Parse player lines (remaining lines)
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		player, err := parsePlayerLine(line)
		if err != nil {
			continue // Skip malformed player lines
		}

		status.Players = append(status.Players, player)
		if player.IsBot {
			status.BotCount++
		} else {
			status.HumanCount++
		}
	}

	// Overlay g_objStatus tail onto matching players (tail meaning is
	// gametype-specific). Unmatched cn (slot churn) drops silently.
	if len(objTail) > 0 {
		for i := range status.Players {
			count, ok := objTail[status.Players[i].ClientNum]
			if !ok {
				continue
			}
			switch status.GameType {
			case "harvester":
				status.Players[i].SkullsCarrying = count
			case "overload":
				status.Players[i].ObelisksDestroyed = count
			}
		}
	}

	return status, nil
}

// parseVars parses backslash-separated key/value pairs
// Format: \key1\value1\key2\value2...
func parseVars(line string) map[string]string {
	vars := make(map[string]string)
	parts := strings.Split(line, "\\")

	// Skip first empty part if line starts with \
	start := 0
	if len(parts) > 0 && parts[0] == "" {
		start = 1
	}

	for i := start; i+1 < len(parts); i += 2 {
		key := strings.ToLower(parts[i])
		value := parts[i+1]
		vars[key] = value
	}

	return vars
}

// parsePlayerLine parses a player line from the status response
// Format: <score> <ping> <team> "<name>" [<clientNum>]
// The clientNum is optional and appended by our modified quake3e server
func parsePlayerLine(line string) (domain.PlayerStatus, error) {
	var player domain.PlayerStatus
	player.ClientNum = -1 // Default if not present (unmodified server)

	// Find the quoted name
	quoteStart := strings.Index(line, "\"")
	quoteEnd := strings.LastIndex(line, "\"")
	if quoteStart == -1 || quoteEnd <= quoteStart {
		return player, fmt.Errorf("no quoted name found")
	}

	player.Name = line[quoteStart+1 : quoteEnd]
	player.CleanName = domain.CleanQ3Name(player.Name)

	// Parse score, ping, and optionally team from the part before the name
	parts := strings.Fields(line[:quoteStart])
	if len(parts) >= 2 {
		player.Score, _ = strconv.Atoi(parts[0])
		player.Ping, _ = strconv.Atoi(parts[1])
	}
	// Some Q3 implementations include team as third field
	if len(parts) >= 3 {
		player.Team, _ = strconv.Atoi(parts[2])
	}

	// Parse optional trailing clientNum (after the closing quote)
	// Format: ... "<name>" <clientNum>
	if quoteEnd+1 < len(line) {
		remainder := strings.TrimSpace(line[quoteEnd+1:])
		if remainder != "" {
			if cn, err := strconv.Atoi(remainder); err == nil {
				player.ClientNum = cn
			}
		}
	}

	// Note: IsBot is set by manager based on GUID presence from log tracking
	// (ping == 0 is not reliable for bot detection on LAN)

	return player, nil
}

// isTeamGameType returns true if the game type is a team-based mode
func isTeamGameType(gameType string) bool {
	switch gameType {
	case "Team Deathmatch", "TDM", "Capture the Flag", "CTF", "One Flag CTF", "Overload", "Harvester",
		"tdm", "ctf", "1fctf", "overload", "harvester": // lowercase variants from GameTypeFromInt()
		return true
	default:
		return false
	}
}

// parseIntVar tries to parse an int from multiple possible var names
func parseIntVar(vars map[string]string, names ...string) (int, bool) {
	for _, name := range names {
		if val, ok := vars[name]; ok {
			if i, err := strconv.Atoi(val); err == nil {
				return i, true
			}
		}
	}
	return 0, false
}

// parseObjStatus parses g_objStatus dispatched by gametype, plus the
// optional "cn:count,..." pipe-tail. Grammar contract:
// trinity/code/game/g_main.c at the cvar registration site.
func parseObjStatus(s, gameType string) (*domain.ObjStatus, map[int]int) {
	var teamPart, tailPart string
	if idx := strings.Index(s, "|"); idx >= 0 {
		teamPart = s[:idx]
		tailPart = s[idx+1:]
	} else {
		teamPart = s
	}

	tail := parseClientCountTail(tailPart)

	switch gameType {
	case "ctf":
		return parseCTFTeamSection(teamPart), tail
	case "1fctf":
		return parse1FCTFTeamSection(teamPart), tail
	case "overload":
		return parseOverloadTeamSection(teamPart), tail
	case "harvester":
		return parseHarvesterTeamSection(teamPart), tail
	default:
		return nil, nil
	}
}

func parseCTFTeamSection(s string) *domain.ObjStatus {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return nil
	}
	redParts := strings.Split(parts[0], ":")
	blueParts := strings.Split(parts[1], ":")
	if len(redParts) != 2 || len(blueParts) != 2 {
		return nil
	}
	redStatus, err1 := strconv.Atoi(redParts[0])
	redCarrier, err2 := strconv.Atoi(redParts[1])
	blueStatus, err3 := strconv.Atoi(blueParts[0])
	blueCarrier, err4 := strconv.Atoi(blueParts[1])
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return nil
	}
	return &domain.ObjStatus{
		Mode:        "ctf",
		Red:         redStatus,
		RedCarrier:  redCarrier,
		Blue:        blueStatus,
		BlueCarrier: blueCarrier,
	}
}

func parse1FCTFTeamSection(s string) *domain.ObjStatus {
	segment := strings.Split(s, ":")
	if len(segment) != 2 {
		return nil
	}
	status, err1 := strconv.Atoi(segment[0])
	carrier, err2 := strconv.Atoi(segment[1])
	if err1 != nil || err2 != nil {
		return nil
	}
	// Normalize stale carrier slot to -1 except when flag is actively held/dropped.
	if status != 2 && status != 3 {
		carrier = -1
	}
	return &domain.ObjStatus{
		Mode:           "1fctf",
		Neutral:        status,
		NeutralCarrier: carrier,
		RedCarrier:     -1, // -1 not 0 so UI per-player lookup can't match cn=0
		BlueCarrier:    -1,
	}
}

func parseOverloadTeamSection(s string) *domain.ObjStatus {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return nil
	}
	redHP, err1 := strconv.Atoi(parts[0])
	blueHP, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return nil
	}
	return &domain.ObjStatus{
		Mode:          "overload",
		RedObeliskHP:  redHP,
		BlueObeliskHP: blueHP,
	}
}

func parseHarvesterTeamSection(s string) *domain.ObjStatus {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return nil
	}
	red, err1 := strconv.Atoi(parts[0])
	blue, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return nil
	}
	return &domain.ObjStatus{
		Mode:       "harvester",
		RedSkulls:  red,
		BlueSkulls: blue,
	}
}

// parseClientCountTail parses "cn:count,cn:count,..." into a map.
// Malformed entries skip; empty input returns nil.
func parseClientCountTail(s string) map[int]int {
	if s == "" {
		return nil
	}
	out := make(map[int]int)
	for _, pair := range strings.Split(s, ",") {
		colon := strings.IndexByte(pair, ':')
		if colon < 0 {
			continue
		}
		cn, err1 := strconv.Atoi(pair[:colon])
		count, err2 := strconv.Atoi(pair[colon+1:])
		if err1 != nil || err2 != nil {
			continue
		}
		out[cn] = count
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
