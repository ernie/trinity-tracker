package directory

import (
	"fmt"
	"strings"
)

// OOBHeader is the Quake III out-of-band datagram prefix, four 0xFF
// bytes before the ASCII command. Every directory packet starts with it.
const OOBHeader = "\xff\xff\xff\xff"

// Commands sent and accepted, matching dpmaster and the Trinity engine.
const (
	cmdHeartbeat          = "heartbeat"
	cmdGetinfo            = "getinfo"
	cmdInfoResponse       = "infoResponse"
	cmdGetservers         = "getservers"
	cmdGetserversExt      = "getserversExt"
	cmdGetserversResponse = "getserversResponse"
	cmdGetserversExtResp  = "getserversExtResponse"
)

// Accepted heartbeat tags. ioquake3 and the Trinity engine send
// QuakeArena-1 (HEARTBEAT_FOR_MASTER in q_shared.h); older trees send
// Quake3Arena.
var acceptedHeartbeatTags = map[string]bool{
	"QuakeArena-1": true,
	"Quake3Arena":  true,
}

// EnginePrefix starts the Trinity engine's `engine` infostring field,
// the com_engine value "trinity-engine/<ver>". Stock ioquake3 sends no
// such field, which is enough to reject it.
const EnginePrefix = "trinity-engine/"

// parseOOB strips the 0xFF prefix and returns the command and the rest,
// split at the first space or newline, with trailing CR/LF trimmed.
// ok is false without the OOB header.
func parseOOB(pkt []byte) (cmd, rest string, ok bool) {
	if len(pkt) < len(OOBHeader)+1 {
		return "", "", false
	}
	if string(pkt[:4]) != OOBHeader {
		return "", "", false
	}
	body := string(pkt[4:])
	body = strings.TrimRight(body, "\r\n")
	// The game uses a space (heartbeat tag, getinfo challenge,
	// getservers protocol) or a newline (before infoResponse's
	// infostring).
	for i, r := range body {
		if r == ' ' || r == '\n' {
			return body[:i], body[i+1:], true
		}
	}
	return body, "", true
}

// parseInfostring parses "\k1\v1\k2\v2..." into a map with lowercased
// keys, as the collector's UDP status parsing does.
func parseInfostring(s string) map[string]string {
	out := make(map[string]string)
	parts := strings.Split(s, "\\")
	start := 0
	if len(parts) > 0 && parts[0] == "" {
		start = 1
	}
	for i := start; i+1 < len(parts); i += 2 {
		out[strings.ToLower(parts[i])] = parts[i+1]
	}
	return out
}

func formatGetinfo(challenge string) []byte {
	return []byte(fmt.Sprintf("%s%s %s\n", OOBHeader, cmdGetinfo, challenge))
}
