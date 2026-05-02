package directory

import (
	"encoding/binary"
	"log"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// Sub-MTU chunk size for getservers responses. dpmaster targets ~1400
// bytes per datagram to stay below typical Internet path MTU after the
// IP+UDP overhead.
const maxResponseChunk = 1400

// recordSizeIPv4 / recordSizeIPv6 are the on-the-wire sizes of one
// packed server entry: separator (1) + IP (4 or 16) + port (2).
const (
	recordSizeIPv4 = 1 + 4 + 2
	recordSizeIPv6 = 1 + 16 + 2
)

// queryFilters captures everything we extract from a getservers /
// getserversExt command line. Empty/zero values mean "no constraint
// on this dimension".
type queryFilters struct {
	gamename     string // Ext only
	protocol     int    // 0 means unset/parse-error → match nothing
	gametype     int
	gametypeSet  bool
	includeEmpty bool
	includeFull  bool
	wantV4       bool // Ext: explicit ipv4 token
	wantV6       bool // Ext: explicit ipv6 token
	extended     bool // true if the command was getserversExt
}

// parseGetservers decodes a `getservers` argument string.
//
// Wire form (after the `getservers` token): `<protocol> [filters...]`.
// Filters: `empty`, `full`, `gametype=N`, plus the legacy shorthand
// `ffa`/`tourney`/`team`/`ctf` which set the gametype.
func parseGetservers(arg string) queryFilters {
	f := queryFilters{}
	tokens := strings.Fields(arg)
	for i, tok := range tokens {
		if i == 0 {
			if n, err := strconv.Atoi(tok); err == nil {
				f.protocol = n
			}
			continue
		}
		applyFilterToken(&f, tok)
	}
	return f
}

// parseGetserversExt decodes a `getserversExt` argument string.
//
// Wire form: `<gamename> <protocol> [filters...]`. Filters add `ipv4`,
// `ipv6` to the legacy set.
func parseGetserversExt(arg string) queryFilters {
	f := queryFilters{extended: true}
	tokens := strings.Fields(arg)
	for i, tok := range tokens {
		switch i {
		case 0:
			f.gamename = tok
		case 1:
			if n, err := strconv.Atoi(tok); err == nil {
				f.protocol = n
			}
		default:
			applyFilterToken(&f, tok)
		}
	}
	return f
}

func applyFilterToken(f *queryFilters, tok string) {
	switch strings.ToLower(tok) {
	case "empty":
		f.includeEmpty = true
	case "full":
		f.includeFull = true
	case "ipv4":
		f.wantV4 = true
	case "ipv6":
		f.wantV6 = true
	case "ffa":
		f.gametype = 0
		f.gametypeSet = true
	case "tourney":
		f.gametype = 1
		f.gametypeSet = true
	case "team":
		f.gametype = 3
		f.gametypeSet = true
	case "ctf":
		f.gametype = 4
		f.gametypeSet = true
	default:
		if strings.HasPrefix(strings.ToLower(tok), "gametype=") {
			if n, err := strconv.Atoi(tok[len("gametype="):]); err == nil {
				f.gametype = n
				f.gametypeSet = true
			}
		}
	}
}

// matchEntry decides whether a registered server should appear in the
// response for these filters.
func matchEntry(e regEntry, f queryFilters) bool {
	if f.protocol == 0 || e.protocol != f.protocol {
		return false
	}
	if f.extended && !e.matchGamename(f.gamename) {
		return false
	}
	// IPv4-only `getservers` cannot return v6 entries.
	if !f.extended && e.addr.Addr().Is6() && !e.addr.Addr().Is4In6() {
		return false
	}
	// `getserversExt` with explicit ipv4/ipv6 tokens restricts the family.
	if f.extended && (f.wantV4 || f.wantV6) {
		isV6 := e.addr.Addr().Is6() && !e.addr.Addr().Is4In6()
		if isV6 && !f.wantV6 {
			return false
		}
		if !isV6 && !f.wantV4 {
			return false
		}
	}
	if f.gametypeSet && e.gametype != f.gametype {
		return false
	}
	if !f.includeEmpty && e.clients == 0 {
		return false
	}
	if !f.includeFull && e.maxClients > 0 && e.clients >= e.maxClients {
		return false
	}
	return true
}

// buildResponses packs filtered entries into one or more datagrams.
// Only the final datagram appends `\EOT\0\0\0`. Each datagram is
// prefixed with the OOB header and the response keyword.
func buildResponses(entries []regEntry, f queryFilters) [][]byte {
	keyword := cmdGetserversResponse
	if f.extended {
		keyword = cmdGetserversExtResp
	}
	prefix := []byte(OOBHeader + keyword)
	eot := []byte("\\EOT\x00\x00\x00")

	// Records that fit in one datagram, starting fresh after `prefix`.
	// We over-allocate the first chunk's prefix once; subsequent chunks
	// reuse the same prefix bytes.
	var (
		out   [][]byte
		buf   = append([]byte(nil), prefix...)
		count int
	)
	flush := func() {
		if count == 0 {
			return
		}
		out = append(out, buf)
		buf = append([]byte(nil), prefix...)
		count = 0
	}

	for _, e := range entries {
		var rec []byte
		ip := e.addr.Addr()
		if ip.Is4() || ip.Is4In6() {
			ip4 := ip.As4()
			rec = make([]byte, recordSizeIPv4)
			rec[0] = '\\'
			copy(rec[1:5], ip4[:])
			binary.BigEndian.PutUint16(rec[5:7], e.addr.Port())
		} else {
			ip6 := ip.As16()
			rec = make([]byte, recordSizeIPv6)
			rec[0] = '/'
			copy(rec[1:17], ip6[:])
			binary.BigEndian.PutUint16(rec[17:19], e.addr.Port())
		}
		if len(buf)+len(rec)+len(eot) > maxResponseChunk {
			flush()
		}
		buf = append(buf, rec...)
		count++
	}
	// Final chunk gets EOT appended (even if empty — clients use the
	// EOT-only datagram as "you got everything").
	buf = append(buf, eot...)
	out = append(out, buf)
	return out
}

// handleGetservers / handleGetserversExt run the full pipeline:
// rate-limit, parse, snapshot, filter, pack, send.
func (s *Server) handleGetservers(conn *net.UDPConn, srcAddr netip.AddrPort, arg string, extended bool) {
	s.metrics.getserversRecv.Add(1)

	if !s.ratelimit.Allow(srcAddr.Addr()) {
		s.metrics.rateLimited.Add(1)
		return
	}

	var f queryFilters
	if extended {
		f = parseGetserversExt(arg)
	} else {
		f = parseGetservers(arg)
	}
	if f.protocol == 0 {
		s.metrics.parseErrors.Add(1)
		return
	}

	all := s.registry.Snapshot()
	matched := all[:0]
	for _, e := range all {
		if matchEntry(e, f) {
			matched = append(matched, e)
		}
	}
	chunks := buildResponses(matched, f)
	for _, c := range chunks {
		if err := sendOn(conn, srcAddr, c); err != nil {
			log.Printf("directory: send response to %s: %v", srcAddr, err)
			return
		}
	}
	s.metrics.getserversReplied.Add(1)
}
