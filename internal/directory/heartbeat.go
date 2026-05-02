package directory

import (
	"log"
	"net"
	"net/netip"
	"strings"
)

// handleHeartbeat is invoked when a UDP packet's command is "heartbeat".
// `arg` is everything after the command (the game tag, e.g.
// "QuakeArena-1"). We gate by membership, then issue a getinfo probe
// with a random challenge — only on a valid infoResponse echo will the
// server land in the registry.
func (s *Server) handleHeartbeat(conn *net.UDPConn, srcAddr netip.AddrPort, arg string) {
	s.metrics.heartbeatsRecv.Add(1)

	if !acceptedHeartbeatTags[arg] {
		s.metrics.heartbeatsRejected.Add(1)
		if s.debug {
			log.Printf("directory: heartbeat from %s with unknown tag %q — dropping", srcAddr, arg)
		}
		return
	}

	if _, ok := s.gate.Allow(srcAddr); !ok {
		s.metrics.heartbeatsRejected.Add(1)
		if s.debug {
			log.Printf("directory: heartbeat from %s rejected — not in hub registration", srcAddr)
		}
		return
	}

	c := s.challenges.Issue(srcAddr)
	if c == "" {
		// Capacity reached — caller dropped. This only happens under a
		// flood of distinct source addrs; logging at warn so it's
		// noticeable but not noisy.
		log.Printf("directory: challenge tracker full, dropping heartbeat from %s", srcAddr)
		return
	}
	s.metrics.probesSent.Add(1)
	if err := sendOn(conn, srcAddr, formatGetinfo(c)); err != nil {
		log.Printf("directory: send getinfo to %s: %v", srcAddr, err)
	}
}

// handleInfoResponse parses an infoResponse packet and, if the echoed
// challenge matches what we issued, validates the source into the
// registry.
func (s *Server) handleInfoResponse(srcAddr netip.AddrPort, body string) {
	s.metrics.infoResponsesRecv.Add(1)
	info := parseInfostring(body)
	echoed, ok := info["challenge"]
	if !ok {
		s.metrics.parseErrors.Add(1)
		return
	}
	if !s.challenges.Take(srcAddr, echoed) {
		// Either the challenge doesn't match (spoofed/late) or it expired
		// (server too slow). Either way: drop quietly. Don't decrement
		// any counter — the validations counter only ticks on success.
		return
	}
	if !strings.HasPrefix(info["engine"], EnginePrefix) {
		// Stock ioquake3 (and most forks) have no `engine` field; any
		// server admitted to the directory MUST self-identify as
		// trinity-engine. This prevents an arbitrary registered IP from
		// proxying a non-trinity server onto our list.
		s.metrics.heartbeatsRejected.Add(1)
		if s.debug {
			log.Printf("directory: %s passed challenge but engine=%q does not match prefix %q — rejecting",
				srcAddr, info["engine"], EnginePrefix)
		}
		return
	}
	id, ok := s.gate.Allow(srcAddr)
	if !ok {
		// Membership could have changed between heartbeat and response;
		// treat as if the entry was never there.
		return
	}
	if !s.registry.Upsert(srcAddr, info, id) {
		// Registry at capacity. Already logged inside Upsert? No — log here.
		log.Printf("directory: registry full (max=%d), refusing %s", s.cfg.MaxServers, srcAddr)
		return
	}
	s.metrics.validations.Add(1)
	if s.debug {
		log.Printf("directory: validated %s (server id=%d, protocol=%s, gamename=%s)",
			srcAddr, id, info["protocol"], info["gamename"])
	}
}
