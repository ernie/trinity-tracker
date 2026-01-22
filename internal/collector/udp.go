package collector

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/ernie/trinity-tools/internal/domain"
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
		status.Name = domain.CleanQ3Name(name)
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

	// Extract flag status for CTF
	// New format: "<red_status>:<red_carrier>,<blue_status>:<blue_carrier>"
	// Example: "1:5,0:-1" = red flag taken by client 5, blue flag at base
	// Legacy format: "XY" where X=red status, Y=blue status
	if flagStatus := vars["g_flagstatus"]; flagStatus != "" {
		status.FlagStatus = parseFlagStatus(flagStatus)
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
		"tdm", "ctf": // lowercase variants from GameTypeFromInt()
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

// parseFlagStatus parses the g_flagStatus cvar
// Format: "<red_status>:<red_carrier>,<blue_status>:<blue_carrier>"
// Example: "1:5,0:-1" = red flag taken by client 5, blue flag at base
func parseFlagStatus(s string) *domain.FlagStatus {
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

	return &domain.FlagStatus{
		Red:         redStatus,
		RedCarrier:  redCarrier,
		Blue:        blueStatus,
		BlueCarrier: blueCarrier,
	}
}
