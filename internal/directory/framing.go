package directory

import (
	"fmt"
	"strings"
)

// OOBHeader is the Quake 3 out-of-band datagram prefix: four 0xFF bytes
// followed immediately by the ASCII command. Every directory packet (in
// or out) starts with this.
const OOBHeader = "\xff\xff\xff\xff"

// Command tokens we send or accept on the wire. Values match dpmaster
// and the ioquake3 fork at ../trinity-engine.
const (
	cmdHeartbeat            = "heartbeat"
	cmdGetinfo              = "getinfo"
	cmdInfoResponse         = "infoResponse"
	cmdGetservers           = "getservers"
	cmdGetserversExt        = "getserversExt"
	cmdGetserversResponse   = "getserversResponse"
	cmdGetserversExtResp    = "getserversExtResponse"
)

// Accepted heartbeat tags. `QuakeArena-1` is what ioquake3 (and the
// trinity-engine fork) emit (see code/qcommon/q_shared.h:49).
// `Quake3Arena` shows up in older trees and is harmless to accept.
var acceptedHeartbeatTags = map[string]bool{
	"QuakeArena-1": true,
	"Quake3Arena":  true,
}

// EnginePrefix is what trinity-engine puts in the `engine` infostring
// field (the `com_engine` cvar's value, format "trinity-engine/<ver>").
// Stock ioquake3 has no such field — its absence is enough to reject.
const EnginePrefix = "trinity-engine/"

// parseOOB strips the four 0xFF prefix and returns (command, rest).
// rest is everything after the first whitespace (newline or space) and
// has any trailing newline/CR trimmed. A packet without the OOB header
// is rejected.
func parseOOB(pkt []byte) (cmd, rest string, ok bool) {
	if len(pkt) < len(OOBHeader)+1 {
		return "", "", false
	}
	if string(pkt[:4]) != OOBHeader {
		return "", "", false
	}
	body := string(pkt[4:])
	body = strings.TrimRight(body, "\r\n")
	// Split on first whitespace — Q3 uses either a space (heartbeat tag,
	// getinfo challenge, getservers protocol) or a newline (infoResponse
	// before its infostring).
	for i, r := range body {
		if r == ' ' || r == '\n' {
			return body[:i], body[i+1:], true
		}
	}
	return body, "", true
}

// parseInfostring parses a backslash-separated "\k1\v1\k2\v2..." string
// into a map. Keys are lowercased to match how the rest of trinity
// handles Q3 vars (see internal/collector/udp.go:200). Empty input
// yields an empty map.
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

// formatGetinfo builds an outbound getinfo probe with the given
// challenge string.
func formatGetinfo(challenge string) []byte {
	return []byte(fmt.Sprintf("%s%s %s\n", OOBHeader, cmdGetinfo, challenge))
}
