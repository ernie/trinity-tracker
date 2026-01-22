package collector

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LogEvent represents a parsed event from the log
type LogEvent struct {
	Timestamp time.Time
	Type      string
	Data      interface{}
}

// Event types
const (
	EventTypeInitGame         = "init_game"
	EventTypeWarmupEnd        = "warmup_end"
	EventTypeWarmup           = "warmup"
	EventTypeMatchState       = "match_state"
	EventTypeClientConnect    = "client_connect"
	EventTypeClientUserinfo   = "client_userinfo"
	EventTypeClientBegin      = "client_begin"
	EventTypeClientDisconnect = "client_disconnect"
	EventTypeKill             = "kill"
	EventTypeExit             = "exit"
	EventTypeScore            = "score"
	EventTypeShutdown         = "shutdown"
	EventTypeBroadcast        = "broadcast"
	EventTypeSpawn            = "spawn"
	EventTypeFlagCapture      = "flag_capture"
	EventTypeFlagTaken        = "flag_taken"
	EventTypeFlagReturn       = "flag_return"
	EventTypeFlagDrop         = "flag_drop"
	EventTypeObeliskDestroy   = "obelisk_destroy"
	EventTypeSkullPickup      = "skull_pickup"
	EventTypeSkullScore       = "skull_score"
	EventTypeTeamChange       = "team_change"
	EventTypeAssist           = "assist"
	EventTypeAward            = "award"
	EventTypeSay              = "say"
	EventTypeSayTeam          = "say_team"
	EventTypeTell             = "tell"
	EventTypeSayRcon          = "say_rcon"
	EventTypeServerStartup    = "server_startup"
	EventTypeServerShutdown   = "server_shutdown"
)

// Event data structures
type InitGameData struct {
	MapName  string
	GameType int
	UUID     string // unique match identifier from game
	Settings map[string]string
}

type ClientConnectData struct {
	ClientID int
}

type ClientUserinfoData struct {
	ClientID int
	Name     string
	Team     int
	Model    string
	IsBot    bool
	Skill    float64 // Bot skill level (1-5), 0 if not a bot
	GUID     string
	Userinfo map[string]string
}

type ClientDisconnectData struct {
	ClientID int
	GUID     string // optional, only present for human players (Trinity)
}

type KillEventData struct {
	KillerID   int
	VictimID   int
	WeaponID   int
	KillerName string
	VictimName string
	Weapon     string
}

type ExitEventData struct {
	Reason    string
	UUID      string // unique match identifier from game
	RedScore  *int   // team scores (nil for non-team games)
	BlueScore *int
}

type ShutdownGameData struct {
	UUID string // unique match identifier from game
}

type ScoreEventData struct {
	Score    int
	Ping     int
	Team     int
	ClientID int
	Name     string
}

type BroadcastData struct {
	Message string
}

type WarmupData struct {
	Duration int // warmup duration in seconds
}

type MatchStateData struct {
	State    string // "waiting", "warmup", "active", "intermission"
	Duration int    // warmup duration in seconds (only for warmup state)
}

type SpawnData struct {
	ClientID int
	Name     string
}

type FlagCaptureData struct {
	ClientID int
	Team     int
	Name     string
}

type FlagTakenData struct {
	ClientID int
	Team     int // team of the flag that was taken
	Name     string
}

type FlagReturnData struct {
	ClientID int
	Team     int
	Name     string // may be empty for auto-return
}

type FlagDropData struct {
	ClientID int
	Team     int
	Name     string
}

type ObeliskDestroyData struct {
	Team       int
	AttackerID int
	Attacker   string
}

type SkullPickupData struct {
	ClientID int
	Team     int
	Count    int // total skulls held after pickup
	Name     string
}

type SkullScoreData struct {
	ClientID int
	Team     int
	Skulls   int // skulls scored
	Name     string
}

type TeamChangeData struct {
	ClientID int
	OldTeam  int
	NewTeam  int
	Name     string
}

type AssistData struct {
	ClientID   int
	Team       int
	AssistType string // "return" or "frag"
	Name       string
}

type AwardData struct {
	ClientID  int
	AwardType string // "impressive" or "excellent"
	Name      string
}

type SayData struct {
	ClientID int
	Name     string
	Message  string
}

type SayTeamData struct {
	ClientID int
	Name     string
	Message  string
}

type TellData struct {
	FromClientID int
	ToClientID   int
	FromName     string
	ToName       string
	Message      string
}

type SayRconData struct {
	Message string
}

// Regular expressions for parsing log lines
var (
	// Matches ISO 8601 timestamp at start of line: 2026-01-12T10:58:23 or 2026-01-12T10:58:23.456789Z
	timestampRegex = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z?)\s+`)

	// Event patterns (after timestamp is stripped)
	initGameRegex         = regexp.MustCompile(`^InitGame: (.+)$`)
	warmupEndRegex        = regexp.MustCompile(`^WarmupEnd:$`)
	warmupRegex           = regexp.MustCompile(`^Warmup: (\d+)$`)
	matchStateRegex       = regexp.MustCompile(`^MatchState: (\w+)(?: (\d+))?$`)
	clientConnectRegex    = regexp.MustCompile(`^ClientConnect: (\d+)$`)
	clientUserinfoRegex   = regexp.MustCompile(`^ClientUserinfoChanged: (\d+) (.+)$`)
	clientBeginRegex      = regexp.MustCompile(`^ClientBegin: (\d+)$`)
	clientDisconnectRegex = regexp.MustCompile(`^ClientDisconnect: (\d+)(?: (.+))?$`)
	killRegex             = regexp.MustCompile(`^Kill: (\d+) (\d+) (\d+): (.+) killed (.+) by (.+)$`)
	exitRegex             = regexp.MustCompile(`^Exit: (.+)$`)
	scoreRegex            = regexp.MustCompile(`^score: (-?\d+)\s+ping: (\d+)\s+team: (\d+)\s+client: (\d+) (.+)$`)
	shutdownRegex         = regexp.MustCompile(`^ShutdownGame:(.*)$`)
	broadcastRegex        = regexp.MustCompile(`^broadcast: print "(.+)"$`)
	spawnRegex            = regexp.MustCompile(`^Spawn: (\d+): (.+)$`)
	flagCaptureRegex      = regexp.MustCompile(`^FlagCapture: (\d+) (\d+): (.+)$`)
	flagTakenRegex        = regexp.MustCompile(`^FlagTaken: (\d+) (\d+): (.+)$`)
	flagReturnRegex       = regexp.MustCompile(`^FlagReturn: (-?\d+) (\d+): (.*)$`)
	flagDropRegex         = regexp.MustCompile(`^FlagDrop: (\d+) (\d+): (.*)$`)
	obeliskDestroyRegex   = regexp.MustCompile(`^ObeliskDestroy: (\d+) (-?\d+): (.*)$`)
	skullPickupRegex      = regexp.MustCompile(`^SkullPickup: (\d+) (\d+) (\d+): (.+)$`)
	skullScoreRegex       = regexp.MustCompile(`^SkullScore: (\d+) (\d+) (\d+): (.+)$`)
	teamChangeRegex       = regexp.MustCompile(`^TeamChange: (\d+) (\d+) (\d+): (.+)$`)
	assistRegex           = regexp.MustCompile(`^Assist: (\d+) (\d+) (return|frag): (.+)$`)
	awardRegex            = regexp.MustCompile(`^Award: (\d+) (impressive|excellent|gauntlet|defend|assist): (.+)$`)
	// Chat patterns: Say: <clientID> "<name>": <message>
	sayRegex              = regexp.MustCompile(`^Say: (\d+) "(.+)": (.+)$`)
	sayTeamRegex          = regexp.MustCompile(`^SayTeam: (\d+) "(.+)": (.+)$`)
	tellRegex             = regexp.MustCompile(`^Tell: (\d+) (\d+) "(.+)" "(.+)": (.+)$`)
	sayRconRegex          = regexp.MustCompile(`^SayRcon: (.+)$`)
	serverStartupRegex    = regexp.MustCompile(`^ServerStartup:$`)
	serverShutdownRegex   = regexp.MustCompile(`^ServerShutdown:$`)
)

// LogTailer watches a log file and parses events
type LogTailer struct {
	path       string
	file       *os.File
	position   int64
	Events     chan LogEvent
	Errors     chan error
	done       chan struct{}
	startAfter *time.Time // if set, replay events after this timestamp on start
}

// NewLogTailer creates a new log tailer
func NewLogTailer(path string, startAfter *time.Time) *LogTailer {
	return &LogTailer{
		path:       path,
		Events:     make(chan LogEvent, 100),
		Errors:     make(chan error, 10),
		done:       make(chan struct{}),
		startAfter: startAfter,
	}
}

// OpenFile opens the log file for reading (used before ReplayFromTimestamp)
func (t *LogTailer) OpenFile() (*os.File, error) {
	file, err := os.Open(t.path)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}
	t.file = file
	return file, nil
}

// Start begins tailing the log file from current position
func (t *LogTailer) Start() error {
	// Open file if not already open
	if t.file == nil {
		file, err := os.Open(t.path)
		if err != nil {
			return fmt.Errorf("opening log file: %w", err)
		}
		t.file = file
	}

	// Only seek to end if no replay was done (position is 0)
	// If replay was done, continue from where it left off
	if t.position == 0 {
		pos, err := t.file.Seek(0, io.SeekEnd)
		if err != nil {
			t.file.Close()
			return fmt.Errorf("seeking to end: %w", err)
		}
		t.position = pos
	}

	go t.tailLoop()
	return nil
}

// ReplayFromTimestamp reads the file from the beginning and calls handler for each event.
// Events with timestamp <= after are passed with replayMode=true (state rebuild only, no DB/events).
// Events with timestamp > after are passed with replayMode=false (full processing).
// This processes events synchronously to avoid database lock contention during startup.
func (t *LogTailer) ReplayFromTimestamp(after time.Time, handler func(LogEvent, bool)) error {
	reader := bufio.NewReader(t.file)
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading line: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		event, err := ParseLine(line)
		if err == nil && event != nil {
			// replayMode=true for events we've already processed (state rebuild only)
			// replayMode=false for new events (full processing with DB/events)
			replayMode := !event.Timestamp.After(after)
			handler(*event, replayMode)
		}
	}

	// Update position to current location (end of file after replay)
	pos, _ := t.file.Seek(0, io.SeekCurrent)
	t.position = pos
	return nil
}

// Stop stops the tailer
func (t *LogTailer) Stop() {
	close(t.done)
	if t.file != nil {
		t.file.Close()
	}
}

// tailLoop continuously reads new content from the log
func (t *LogTailer) tailLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-t.done:
			return
		case <-ticker.C:
			if err := t.readNewContent(); err != nil {
				select {
				case t.Errors <- err:
				default:
				}
			}
		}
	}
}

// readNewContent reads any new content since last read
func (t *LogTailer) readNewContent() error {
	stat, err := t.file.Stat()
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	// Handle copytruncate: file size smaller than position
	if stat.Size() < t.position {
		t.position = 0
		if _, err := t.file.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("seeking to start after truncate: %w", err)
		}
	}

	// No new content
	if stat.Size() == t.position {
		return nil
	}

	// Read new content
	reader := bufio.NewReader(t.file)
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			// Partial line - don't advance position past it
			break
		}
		if err != nil {
			return fmt.Errorf("reading line: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		event, err := ParseLine(line)
		if err == nil && event != nil {
			select {
			case t.Events <- *event:
			default:
				// Channel full, drop event
			}
		}
	}

	// Update position
	pos, _ := t.file.Seek(0, io.SeekCurrent)
	t.position = pos

	return nil
}

// ParseLine parses a single log line into an event
func ParseLine(line string) (*LogEvent, error) {
	var timestamp time.Time
	content := line

	// Try to extract ISO 8601 timestamp
	if match := timestampRegex.FindStringSubmatch(line); match != nil {
		// Try parsing with timezone, then without
		ts, err := time.Parse(time.RFC3339Nano, match[1])
		if err != nil {
			// Try without timezone (local time format: 2006-01-02T15:04:05)
			ts, err = time.ParseInLocation("2006-01-02T15:04:05", match[1], time.Local)
		}
		if err == nil {
			timestamp = ts
			content = line[len(match[0]):]
		}
	}

	// If no timestamp, use current time
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	// Try to match event patterns
	event := &LogEvent{Timestamp: timestamp}

	if match := initGameRegex.FindStringSubmatch(content); match != nil {
		settings := parseUserinfo(match[1])
		gameType := 0
		if gt, ok := settings["g_gametype"]; ok {
			gameType, _ = strconv.Atoi(gt)
		}
		// Extract match UUID if present
		matchUUID := settings["g_matchUUID"]
		delete(settings, "g_matchUUID") // Don't include in settings map

		event.Type = EventTypeInitGame
		event.Data = InitGameData{
			MapName:  settings["mapname"],
			GameType: gameType,
			UUID:     matchUUID,
			Settings: settings,
		}
		return event, nil
	}

	if warmupEndRegex.MatchString(content) {
		event.Type = EventTypeWarmupEnd
		return event, nil
	}

	if match := warmupRegex.FindStringSubmatch(content); match != nil {
		duration, _ := strconv.Atoi(match[1])
		event.Type = EventTypeWarmup
		event.Data = WarmupData{Duration: duration}
		return event, nil
	}

	if match := matchStateRegex.FindStringSubmatch(content); match != nil {
		duration := 0
		if match[2] != "" {
			duration, _ = strconv.Atoi(match[2])
		}
		event.Type = EventTypeMatchState
		event.Data = MatchStateData{
			State:    match[1],
			Duration: duration,
		}
		return event, nil
	}

	if match := clientConnectRegex.FindStringSubmatch(content); match != nil {
		clientID, _ := strconv.Atoi(match[1])
		event.Type = EventTypeClientConnect
		event.Data = ClientConnectData{ClientID: clientID}
		return event, nil
	}

	if match := clientUserinfoRegex.FindStringSubmatch(content); match != nil {
		clientID, _ := strconv.Atoi(match[1])
		userinfo := parseUserinfo(match[2])

		team := 0
		if t, ok := userinfo["t"]; ok {
			team, _ = strconv.Atoi(t)
		}

		// Bot detection: presence of "skill" field
		skillStr, isBot := userinfo["skill"]
		var skill float64
		if isBot {
			skill, _ = strconv.ParseFloat(skillStr, 64)
		}

		// Use hmodel (head model) for portraits if available, otherwise fall back to model
		// In Team Arena, hmodel can be different from model (e.g., model=janet, hmodel=*gammy)
		model := userinfo["hmodel"]
		if model == "" {
			model = userinfo["model"]
		}

		event.Type = EventTypeClientUserinfo
		event.Data = ClientUserinfoData{
			ClientID: clientID,
			Name:     userinfo["n"],
			Team:     team,
			Model:    model,
			IsBot:    isBot,
			Skill:    skill,
			GUID:     userinfo["g"],
			Userinfo: userinfo,
		}
		return event, nil
	}

	if match := clientBeginRegex.FindStringSubmatch(content); match != nil {
		clientID, _ := strconv.Atoi(match[1])
		event.Type = EventTypeClientBegin
		event.Data = ClientConnectData{ClientID: clientID}
		return event, nil
	}

	if match := clientDisconnectRegex.FindStringSubmatch(content); match != nil {
		clientID, _ := strconv.Atoi(match[1])
		guid := ""
		if len(match) > 2 && match[2] != "" {
			guid = match[2]
		}
		event.Type = EventTypeClientDisconnect
		event.Data = ClientDisconnectData{ClientID: clientID, GUID: guid}
		return event, nil
	}

	if match := killRegex.FindStringSubmatch(content); match != nil {
		killerID, _ := strconv.Atoi(match[1])
		victimID, _ := strconv.Atoi(match[2])
		weaponID, _ := strconv.Atoi(match[3])
		event.Type = EventTypeKill
		event.Data = KillEventData{
			KillerID:   killerID,
			VictimID:   victimID,
			WeaponID:   weaponID,
			KillerName: match[4],
			VictimName: match[5],
			Weapon:     match[6],
		}
		return event, nil
	}

	if match := exitRegex.FindStringSubmatch(content); match != nil {
		// Parse Exit: <reason> \g_matchUUID\<uuid>[\g_redScore\<red>\g_blueScore\<blue>]
		exitContent := match[1]
		reason := exitContent
		uuid := ""
		var redScore, blueScore *int

		// Check if there are key-value pairs appended (format: "reason \key\value\key\value...")
		if idx := strings.Index(exitContent, "\\g_matchUUID\\"); idx != -1 {
			reason = strings.TrimSpace(exitContent[:idx])
			kvPart := exitContent[idx+1:] // Skip the leading backslash
			kvPairs := parseUserinfo(kvPart)
			uuid = kvPairs["g_matchUUID"]
			if rs, ok := kvPairs["g_redScore"]; ok {
				if v, err := strconv.Atoi(rs); err == nil {
					redScore = &v
				}
			}
			if bs, ok := kvPairs["g_blueScore"]; ok {
				if v, err := strconv.Atoi(bs); err == nil {
					blueScore = &v
				}
			}
		}

		event.Type = EventTypeExit
		event.Data = ExitEventData{Reason: reason, UUID: uuid, RedScore: redScore, BlueScore: blueScore}
		return event, nil
	}

	if match := scoreRegex.FindStringSubmatch(content); match != nil {
		score, _ := strconv.Atoi(match[1])
		ping, _ := strconv.Atoi(match[2])
		team, _ := strconv.Atoi(match[3])
		clientID, _ := strconv.Atoi(match[4])
		event.Type = EventTypeScore
		event.Data = ScoreEventData{
			Score:    score,
			Ping:     ping,
			Team:     team,
			ClientID: clientID,
			Name:     match[5],
		}
		return event, nil
	}

	if match := shutdownRegex.FindStringSubmatch(content); match != nil {
		// Parse ShutdownGame: \g_matchUUID\<uuid> (or empty for legacy logs)
		uuid := ""
		if len(match) > 1 {
			shutdownContent := strings.TrimSpace(match[1])
			if strings.HasPrefix(shutdownContent, "\\g_matchUUID\\") {
				uuid = shutdownContent[len("\\g_matchUUID\\"):]
			}
		}
		event.Type = EventTypeShutdown
		event.Data = ShutdownGameData{UUID: uuid}
		return event, nil
	}

	if match := broadcastRegex.FindStringSubmatch(content); match != nil {
		event.Type = EventTypeBroadcast
		event.Data = BroadcastData{Message: match[1]}
		return event, nil
	}

	if match := spawnRegex.FindStringSubmatch(content); match != nil {
		clientID, _ := strconv.Atoi(match[1])
		event.Type = EventTypeSpawn
		event.Data = SpawnData{
			ClientID: clientID,
			Name:     match[2],
		}
		return event, nil
	}

	if match := flagCaptureRegex.FindStringSubmatch(content); match != nil {
		clientID, _ := strconv.Atoi(match[1])
		team, _ := strconv.Atoi(match[2])
		event.Type = EventTypeFlagCapture
		event.Data = FlagCaptureData{
			ClientID: clientID,
			Team:     team,
			Name:     match[3],
		}
		return event, nil
	}

	if match := flagTakenRegex.FindStringSubmatch(content); match != nil {
		clientID, _ := strconv.Atoi(match[1])
		team, _ := strconv.Atoi(match[2])
		event.Type = EventTypeFlagTaken
		event.Data = FlagTakenData{
			ClientID: clientID,
			Team:     team,
			Name:     match[3],
		}
		return event, nil
	}

	if match := flagReturnRegex.FindStringSubmatch(content); match != nil {
		clientID, _ := strconv.Atoi(match[1])
		team, _ := strconv.Atoi(match[2])
		event.Type = EventTypeFlagReturn
		event.Data = FlagReturnData{
			ClientID: clientID,
			Team:     team,
			Name:     match[3],
		}
		return event, nil
	}

	if match := flagDropRegex.FindStringSubmatch(content); match != nil {
		clientID, _ := strconv.Atoi(match[1])
		team, _ := strconv.Atoi(match[2])
		event.Type = EventTypeFlagDrop
		event.Data = FlagDropData{
			ClientID: clientID,
			Team:     team,
			Name:     match[3],
		}
		return event, nil
	}

	if match := obeliskDestroyRegex.FindStringSubmatch(content); match != nil {
		team, _ := strconv.Atoi(match[1])
		attackerID, _ := strconv.Atoi(match[2])
		event.Type = EventTypeObeliskDestroy
		event.Data = ObeliskDestroyData{
			Team:       team,
			AttackerID: attackerID,
			Attacker:   match[3],
		}
		return event, nil
	}

	if match := skullPickupRegex.FindStringSubmatch(content); match != nil {
		clientID, _ := strconv.Atoi(match[1])
		team, _ := strconv.Atoi(match[2])
		count, _ := strconv.Atoi(match[3])
		event.Type = EventTypeSkullPickup
		event.Data = SkullPickupData{
			ClientID: clientID,
			Team:     team,
			Count:    count,
			Name:     match[4],
		}
		return event, nil
	}

	if match := skullScoreRegex.FindStringSubmatch(content); match != nil {
		clientID, _ := strconv.Atoi(match[1])
		team, _ := strconv.Atoi(match[2])
		skulls, _ := strconv.Atoi(match[3])
		event.Type = EventTypeSkullScore
		event.Data = SkullScoreData{
			ClientID: clientID,
			Team:     team,
			Skulls:   skulls,
			Name:     match[4],
		}
		return event, nil
	}

	if match := teamChangeRegex.FindStringSubmatch(content); match != nil {
		clientID, _ := strconv.Atoi(match[1])
		oldTeam, _ := strconv.Atoi(match[2])
		newTeam, _ := strconv.Atoi(match[3])
		event.Type = EventTypeTeamChange
		event.Data = TeamChangeData{
			ClientID: clientID,
			OldTeam:  oldTeam,
			NewTeam:  newTeam,
			Name:     match[4],
		}
		return event, nil
	}

	if match := assistRegex.FindStringSubmatch(content); match != nil {
		clientID, _ := strconv.Atoi(match[1])
		team, _ := strconv.Atoi(match[2])
		event.Type = EventTypeAssist
		event.Data = AssistData{
			ClientID:   clientID,
			Team:       team,
			AssistType: match[3],
			Name:       match[4],
		}
		return event, nil
	}

	if match := awardRegex.FindStringSubmatch(content); match != nil {
		clientID, _ := strconv.Atoi(match[1])
		event.Type = EventTypeAward
		event.Data = AwardData{
			ClientID:  clientID,
			AwardType: match[2],
			Name:      match[3],
		}
		return event, nil
	}

	if match := sayRegex.FindStringSubmatch(content); match != nil {
		clientID, _ := strconv.Atoi(match[1])
		event.Type = EventTypeSay
		event.Data = SayData{
			ClientID: clientID,
			Name:     match[2],
			Message:  match[3],
		}
		return event, nil
	}

	if match := sayTeamRegex.FindStringSubmatch(content); match != nil {
		clientID, _ := strconv.Atoi(match[1])
		event.Type = EventTypeSayTeam
		event.Data = SayTeamData{
			ClientID: clientID,
			Name:     match[2],
			Message:  match[3],
		}
		return event, nil
	}

	if match := tellRegex.FindStringSubmatch(content); match != nil {
		fromClientID, _ := strconv.Atoi(match[1])
		toClientID, _ := strconv.Atoi(match[2])
		event.Type = EventTypeTell
		event.Data = TellData{
			FromClientID: fromClientID,
			ToClientID:   toClientID,
			FromName:     match[3],
			ToName:       match[4],
			Message:      match[5],
		}
		return event, nil
	}

	if match := sayRconRegex.FindStringSubmatch(content); match != nil {
		event.Type = EventTypeSayRcon
		event.Data = SayRconData{
			Message: match[1],
		}
		return event, nil
	}

	if serverStartupRegex.MatchString(content) {
		event.Type = EventTypeServerStartup
		return event, nil
	}

	if serverShutdownRegex.MatchString(content) {
		event.Type = EventTypeServerShutdown
		return event, nil
	}

	// Unknown event type
	return nil, fmt.Errorf("unknown event: %s", content)
}

// parseUserinfo parses backslash-separated userinfo string
// Format is \key\value\key\value (starts with backslash)
func parseUserinfo(info string) map[string]string {
	result := make(map[string]string)
	parts := strings.Split(info, "\\")

	// Skip empty first element if string starts with backslash
	start := 0
	if len(parts) > 0 && parts[0] == "" {
		start = 1
	}

	for i := start; i+1 < len(parts); i += 2 {
		result[parts[i]] = parts[i+1]
	}

	return result
}
