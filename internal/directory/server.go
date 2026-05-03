// Package directory implements a Quake 3 directory (a.k.a. master) server.
//
// Heartbeats are accepted only from servers that are already
// registered on the trinity hub: their stored address (after DNS
// resolution) must match the heartbeat's UDP source IP:port. Once an
// admitted server completes the standard dpmaster-style challenge /
// infoResponse round-trip, it appears in subsequent getservers and
// getserversExt responses to clients.
package directory

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"sync"
	"time"
)

// Config bundles the runtime knobs. All durations are required to be
// positive — the caller (config.DirectoryConfig + applyTrackerDefaults)
// supplies sensible defaults.
type Config struct {
	ListenAddr       string
	Port             int
	HeartbeatExpiry  time.Duration
	ChallengeTimeout time.Duration
	GateRefresh      time.Duration
	MaxServers       int

	Store gateStore
	Conns gateConns

	// Debug enables per-packet logging at info level. Useful during
	// rollout; off in production.
	Debug bool
}

// Server is the Q3 directory server. Construct with New, then call Run
// (blocking) and Stop.
type Server struct {
	cfg        Config
	debug      bool
	conns      []*net.UDPConn
	gate       *gate
	challenges *challengeTracker
	registry   *registry
	ratelimit  *rateLimiter
	metrics    *metrics

	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// New validates cfg and constructs a Server. It does not bind sockets
// yet — that happens in Run, so the caller can defer Stop unconditionally.
func New(cfg Config) (*Server, error) {
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return nil, fmt.Errorf("directory: invalid port %d", cfg.Port)
	}
	if cfg.HeartbeatExpiry <= 0 {
		return nil, fmt.Errorf("directory: HeartbeatExpiry must be positive")
	}
	if cfg.ChallengeTimeout <= 0 {
		return nil, fmt.Errorf("directory: ChallengeTimeout must be positive")
	}
	if cfg.GateRefresh <= 0 {
		return nil, fmt.Errorf("directory: GateRefresh must be positive")
	}
	if cfg.MaxServers <= 0 {
		return nil, fmt.Errorf("directory: MaxServers must be positive")
	}
	if cfg.Store == nil {
		return nil, errors.New("directory: Store is required")
	}
	if cfg.Conns == nil {
		return nil, errors.New("directory: Conns is required")
	}
	return &Server{
		cfg:        cfg,
		debug:      cfg.Debug,
		gate:       newGate(cfg.Store, cfg.Conns, cfg.GateRefresh),
		challenges: newChallengeTracker(cfg.ChallengeTimeout, cfg.MaxServers*2, nil),
		registry:   newRegistry(cfg.HeartbeatExpiry, cfg.MaxServers, nil),
		ratelimit:  newRateLimiter(10, 1.0/3.0, nil),
		metrics:    &metrics{},
		stopCh:     make(chan struct{}),
	}, nil
}

// Run binds the UDP listener(s) and serves until ctx is cancelled or
// Stop is called. The returned error is non-nil only on bind failure
// or unexpected read errors.
func (s *Server) Run(ctx context.Context) error {
	listenAddrs, err := s.bindAddrs()
	if err != nil {
		return err
	}
	for _, addr := range listenAddrs {
		c, err := net.ListenUDP(addr.Network(), addr.UDPAddr())
		if err != nil {
			s.closeConns()
			return fmt.Errorf("directory: listen %s: %w", addr.String(), err)
		}
		s.conns = append(s.conns, c)
		log.Printf("directory: listening on %s", c.LocalAddr())
	}

	gateCtx, gateCancel := context.WithCancel(ctx)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.gate.Run(gateCtx)
	}()

	sweepCtx, sweepCancel := context.WithCancel(ctx)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runSweep(sweepCtx)
	}()

	for _, c := range s.conns {
		s.wg.Add(1)
		go func(conn *net.UDPConn) {
			defer s.wg.Done()
			s.readLoop(conn)
		}(c)
	}

	select {
	case <-ctx.Done():
	case <-s.stopCh:
	}

	gateCancel()
	sweepCancel()
	s.closeConns()
	s.wg.Wait()
	return nil
}

// Stop signals the server to shut down. Idempotent.
func (s *Server) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

// Stats returns a point-in-time snapshot of internal counters.
func (s *Server) Stats() Stats {
	return Stats{
		HeartbeatsReceived:    s.metrics.heartbeatsRecv.Load(),
		HeartbeatsRejected:    s.metrics.heartbeatsRejected.Load(),
		ProbesSent:            s.metrics.probesSent.Load(),
		InfoResponsesReceived: s.metrics.infoResponsesRecv.Load(),
		Validations:           s.metrics.validations.Load(),
		GetserversReceived:    s.metrics.getserversRecv.Load(),
		GetserversReplied:     s.metrics.getserversReplied.Load(),
		RateLimited:           s.metrics.rateLimited.Load(),
		ParseErrors:           s.metrics.parseErrors.Load(),
		RegistrySize:          s.registry.Len(),
		GateSize:              s.gate.Size(),
	}
}

type bindAddr struct {
	ip   netip.Addr
	port uint16
	v6   bool
}

func (b bindAddr) Network() string {
	if b.v6 {
		return "udp6"
	}
	return "udp4"
}

func (b bindAddr) UDPAddr() *net.UDPAddr {
	if !b.ip.IsValid() {
		return &net.UDPAddr{Port: int(b.port)}
	}
	return &net.UDPAddr{IP: b.ip.AsSlice(), Port: int(b.port), Zone: b.ip.Zone()}
}

func (b bindAddr) String() string {
	if !b.ip.IsValid() {
		if b.v6 {
			return fmt.Sprintf("[::]:%d", b.port)
		}
		return fmt.Sprintf("0.0.0.0:%d", b.port)
	}
	return netip.AddrPortFrom(b.ip, b.port).String()
}

// bindAddrs maps cfg.ListenAddr to one or two bind addresses.
//   - "" or "0.0.0.0": IPv4 wildcard only.
//   - "::": IPv6 wildcard, dual-stack default (kernel exposes v4-mapped).
//   - "*" / "any" / "all": dual-stack — listen on both 0.0.0.0 and ::.
//   - Any other literal: parse as a single IP.
func (s *Server) bindAddrs() ([]bindAddr, error) {
	port := uint16(s.cfg.Port)
	la := s.cfg.ListenAddr
	switch la {
	case "", "0.0.0.0":
		return []bindAddr{{port: port, v6: false}}, nil
	case "::":
		return []bindAddr{{ip: netip.IPv6Unspecified(), port: port, v6: true}}, nil
	case "*", "any", "all":
		return []bindAddr{
			{port: port, v6: false},
			{ip: netip.IPv6Unspecified(), port: port, v6: true},
		}, nil
	}
	ip, err := netip.ParseAddr(la)
	if err != nil {
		return nil, fmt.Errorf("directory: invalid listen_addr %q: %w", la, err)
	}
	return []bindAddr{{ip: ip, port: port, v6: ip.Is6() && !ip.Is4In6()}}, nil
}

func (s *Server) closeConns() {
	for _, c := range s.conns {
		_ = c.Close()
	}
	s.conns = nil
}

func (s *Server) runSweep(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n := s.registry.Sweep()
			if n > 0 && s.debug {
				log.Printf("directory: swept %d expired registrations", n)
			}
		}
	}
}

// readLoop pulls datagrams off one UDP socket and dispatches them.
// Each conn gets its own goroutine; sends from heartbeat handlers go
// back through whichever conn the request came in on (so we don't
// have to track address-family routing separately).
func (s *Server) readLoop(conn *net.UDPConn) {
	buf := make([]byte, 65535)
	for {
		n, raddr, err := conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("directory: read on %s: %v", conn.LocalAddr(), err)
			return
		}
		// Normalize v4-in-v6 to plain v4 so heartbeat sources match the
		// gate (which stores plain v4 entries via Unmap()).
		raddr = netip.AddrPortFrom(raddr.Addr().Unmap(), raddr.Port())
		s.dispatch(conn, raddr, buf[:n])
	}
}

// dispatch routes one datagram to the right handler. The conn is
// passed through so the reply leaves on the same socket the request
// arrived on (avoiding any address-family routing surprises with
// dual-stack listeners).
func (s *Server) dispatch(conn *net.UDPConn, srcAddr netip.AddrPort, pkt []byte) {
	cmd, rest, ok := parseOOB(pkt)
	if !ok {
		s.metrics.parseErrors.Add(1)
		return
	}
	switch cmd {
	case cmdHeartbeat:
		s.handleHeartbeat(conn, srcAddr, rest)
	case cmdInfoResponse:
		s.handleInfoResponse(srcAddr, rest)
	case cmdGetservers:
		s.handleGetservers(conn, srcAddr, rest, false)
	case cmdGetserversExt:
		s.handleGetservers(conn, srcAddr, rest, true)
	default:
		// Unknown command. Q3 has many OOB commands; ignoring is safe.
		if s.debug {
			log.Printf("directory: %s sent unknown command %q", srcAddr, cmd)
		}
	}
}

// sendOn transmits one datagram to dst over the supplied conn.
func sendOn(conn *net.UDPConn, dst netip.AddrPort, pkt []byte) error {
	_, err := conn.WriteToUDPAddrPort(pkt, dst)
	return err
}
