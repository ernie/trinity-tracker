// Package directory implements a Quake III directory (master) server.
//
// It accepts heartbeats only from servers registered on the hub, whose
// stored address resolves to the heartbeat's UDP source IP:port. Once a
// server completes the dpmaster-style challenge and infoResponse round
// trip, it's listed in getservers and getserversExt responses.
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

	"github.com/ernie/trinity-tracker/internal/storage"
)

// registryStore is the part of *storage.Store that saves the registry
// across hub restarts.
type registryStore interface {
	ListDirectoryRegistrations(ctx context.Context) ([]storage.DirectoryRegistration, error)
	ReplaceDirectoryRegistrations(ctx context.Context, rows []storage.DirectoryRegistration) error
	ClearDirectoryRegistrations(ctx context.Context) error
}

// Config holds the server's settings. Every duration must be positive;
// config.DirectoryConfig and applyTrackerDefaults supply defaults.
type Config struct {
	ListenAddr       string
	Port             int
	HeartbeatExpiry  time.Duration
	ChallengeTimeout time.Duration
	GateRefresh      time.Duration
	MaxServers       int

	Store gateStore
	Conns gateConns

	// RegistryStore, when set, saves the registry at graceful shutdown
	// and restores it at startup if it's fresher than
	// PersistedFreshness.
	RegistryStore registryStore

	// PersistedFreshness is required with RegistryStore: a snapshot
	// whose newest validated_at is older is discarded.
	PersistedFreshness time.Duration

	// Debug logs every packet at info level.
	Debug bool
}

// Server is the directory server. Construct it with New, then call Run,
// which blocks, and Stop.
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
	// started closes once Run has bound its listeners, restored the
	// saved registry and started its goroutines, so tests can wait for
	// it without polling.
	started chan struct{}
	wg      sync.WaitGroup
}

// New validates cfg and constructs a Server. Run binds the sockets, so
// the caller can always defer Stop.
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
	if cfg.RegistryStore != nil && cfg.PersistedFreshness <= 0 {
		return nil, errors.New("directory: PersistedFreshness must be positive when RegistryStore is set")
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
		started:    make(chan struct{}),
	}, nil
}

// Run binds the UDP listeners and serves until ctx is canceled or Stop
// is called. It returns an error only for a failed bind or an
// unexpected read error.
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

	// Load the gate before the read loops start, so the first heartbeat
	// finds it populated.
	s.gate.refreshOnce(ctx)
	s.restorePersisted(ctx)

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

	close(s.started)

	select {
	case <-ctx.Done():
	case <-s.stopCh:
	}

	gateCancel()
	sweepCancel()
	s.closeConns()
	s.wg.Wait()
	s.persistOnStop()
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

// bindAddrs maps cfg.ListenAddr to one or two bind addresses:
//   - "" or "0.0.0.0": the IPv4 wildcard only.
//   - "::": the IPv6 wildcard, which the kernel makes dual-stack by
//     default with v4-mapped addresses.
//   - "*", "any" or "all": both 0.0.0.0 and ::.
//   - anything else: a single IP.
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

// readLoop reads and dispatches datagrams from one socket, in its own
// goroutine.
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
		// The gate stores plain IPv4, so unmap v4-in-v6 sources.
		raddr = netip.AddrPortFrom(raddr.Addr().Unmap(), raddr.Port())
		s.dispatch(conn, raddr, buf[:n])
	}
}

// dispatch routes one datagram to its handler, passing conn so the
// reply leaves on the socket the request arrived on, with no
// address-family routing to work out on dual-stack listeners.
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
		// The game has many other OOB commands.
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

// restorePersisted loads the saved registry if it's fresh enough, and
// clears a stale one, since its servers may be gone. After an error the
// registry starts empty and refills from heartbeats.
func (s *Server) restorePersisted(ctx context.Context) {
	if s.cfg.RegistryStore == nil {
		return
	}
	rows, err := s.cfg.RegistryStore.ListDirectoryRegistrations(ctx)
	if err != nil {
		log.Printf("directory: list persisted registrations: %v", err)
		return
	}
	keep, clearAll := decideRestore(rows, s.cfg.PersistedFreshness, time.Now())
	if clearAll {
		log.Printf("directory: persisted snapshot of %d entries is stale (>%s), discarding",
			len(rows), s.cfg.PersistedFreshness)
		if err := s.cfg.RegistryStore.ClearDirectoryRegistrations(ctx); err != nil {
			log.Printf("directory: clear stale registrations: %v", err)
		}
		return
	}
	if len(keep) == 0 {
		return
	}
	s.registry.Load(keep)
	log.Printf("directory: restored %d/%d persisted registrations", len(keep), len(rows))
}

// persistOnStop saves the registry after Run's goroutines have stopped,
// so the snapshot is consistent. Shutdown usually comes from canceling
// Run's ctx, so it uses a fresh context; 5s leaves room for SQLite
// contention and stays well under systemd's default TimeoutStopSec.
func (s *Server) persistOnStop() {
	if s.cfg.RegistryStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows := s.registry.toPersisted()
	if err := s.cfg.RegistryStore.ReplaceDirectoryRegistrations(ctx, rows); err != nil {
		log.Printf("directory: persist registry on stop: %v", err)
		return
	}
	if s.debug {
		log.Printf("directory: persisted %d registrations on stop", len(rows))
	}
}
