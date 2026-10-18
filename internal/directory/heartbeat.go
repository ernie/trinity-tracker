package directory

import (
	"log"
	"net"
	"net/netip"
	"strings"
)

// handleHeartbeat handles a "heartbeat" packet; arg is the game tag,
// such as "QuakeArena-1". A member server gets a getinfo probe with a
// random challenge, and only a valid infoResponse echo puts it in the
// registry.
func (s *Server) handleHeartbeat(conn *net.UDPConn, srcAddr netip.AddrPort, arg string) {
	s.metrics.heartbeatsRecv.Add(1)

	if !acceptedHeartbeatTags[arg] {
		s.metrics.heartbeatsRejected.Add(1)
		if s.debug {
			log.Printf("directory: heartbeat from %s with unknown tag %q; dropping", srcAddr, arg)
		}
		return
	}

	if _, ok := s.gate.Allow(srcAddr); !ok {
		s.metrics.heartbeatsRejected.Add(1)
		if s.debug {
			log.Printf("directory: heartbeat from %s rejected: not in hub registration", srcAddr)
		}
		return
	}

	c := s.challenges.Issue(srcAddr)
	if c == "" {
		// Only a flood of distinct source addresses fills the tracker.
		log.Printf("directory: challenge tracker full, dropping heartbeat from %s", srcAddr)
		return
	}
	s.metrics.probesSent.Add(1)
	if err := sendOn(conn, srcAddr, formatGetinfo(c)); err != nil {
		log.Printf("directory: send getinfo to %s: %v", srcAddr, err)
	}
}

// handleInfoResponse adds the sender to the registry when its echoed
// challenge matches the one issued.
func (s *Server) handleInfoResponse(srcAddr netip.AddrPort, body string) {
	s.metrics.infoResponsesRecv.Add(1)
	info := parseInfostring(body)
	echoed, ok := info["challenge"]
	if !ok {
		s.metrics.parseErrors.Add(1)
		return
	}
	if !s.challenges.Take(srcAddr, echoed) {
		// A wrong (spoofed or late) or expired challenge.
		return
	}
	if !strings.HasPrefix(info["engine"], EnginePrefix) {
		// Only the Trinity engine sends this field, so a registered IP
		// can't put some other server on the list.
		s.metrics.heartbeatsRejected.Add(1)
		if s.debug {
			log.Printf("directory: %s passed challenge but engine=%q does not match prefix %q; rejecting",
				srcAddr, info["engine"], EnginePrefix)
		}
		return
	}
	id, ok := s.gate.Allow(srcAddr)
	if !ok {
		// Membership changed between heartbeat and response.
		return
	}
	if !s.registry.Upsert(srcAddr, info, id) {
		log.Printf("directory: registry full (max=%d), refusing %s", s.cfg.MaxServers, srcAddr)
		return
	}
	s.metrics.validations.Add(1)
	if s.debug {
		log.Printf("directory: validated %s (server id=%d, protocol=%s, gamename=%s)",
			srcAddr, id, info["protocol"], info["gamename"])
	}
}
