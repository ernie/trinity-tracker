// Package natsbus owns the embedded NATS server used in hub mode and
// the JetStream streams that carry distributed-tracking traffic.
package natsbus

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/config"
)

const (
	StreamEvents   = "TRINITY_EVENTS"
	StreamRegister = "TRINITY_REGISTER"
)

// EventsMaxBytes is a disk-use guard paired with the age-based limit.
const EventsMaxBytes = 1 * 1024 * 1024 * 1024

// Server owns the embedded nats-server and the in-process management conn.
type Server struct {
	ns       *server.Server
	mgmtConn *nats.Conn
	storeDir string
	url      string
	auth     *AuthStore
}

// Start boots the embedded server and declares the TRINITY streams.
// JetStream state is saved under <storeDirParent>/nats. store keeps
// per-source user pubkeys for AuthStore; tests that never mint or
// revoke may pass nil.
func Start(cfg *config.TrackerConfig, storeDirParent string, store PubKeyStore) (*Server, error) {
	if cfg == nil || cfg.Hub == nil {
		return nil, fmt.Errorf("natsbus.Start: hub config required")
	}

	host, port, err := parseBindAddr(cfg.NATS.URL)
	if err != nil {
		return nil, err
	}

	storeDir := filepath.Join(storeDirParent, "nats")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		return nil, fmt.Errorf("natsbus: creating JetStream store dir %s: %w", storeDir, err)
	}

	auth, err := LoadOrCreateAuthStore(storeDirParent, store)
	if err != nil {
		return nil, err
	}

	opts := &server.Options{
		Host:              host,
		Port:              port,
		JetStream:         true,
		StoreDir:          storeDir,
		JetStreamMaxStore: 2 * EventsMaxBytes,
		NoLog:             true,
		NoSigs:            true,
		TrustedOperators:  auth.TrustedOperators(),
		SystemAccount:     auth.SystemAccountPublicKey(),
		AccountResolver:   auth.Resolver(),
	}

	if cfg.NATS.CertFile != "" || cfg.NATS.KeyFile != "" {
		if cfg.NATS.CertFile == "" || cfg.NATS.KeyFile == "" {
			return nil, fmt.Errorf("natsbus: tracker.nats.cert_file and key_file must both be set or both empty")
		}
		cert, err := tls.LoadX509KeyPair(cfg.NATS.CertFile, cfg.NATS.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("natsbus: loading TLS keypair: %w", err)
		}
		opts.TLS = true
		opts.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("natsbus: NewServer: %w", err)
	}
	ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		ns.Shutdown()
		ns.WaitForShutdown()
		return nil, fmt.Errorf("natsbus: server not ready within 10s")
	}
	auth.AttachServer(ns)

	mgmt, err := nats.Connect("",
		nats.InProcessServer(ns),
		nats.Name("trinity-hub-mgmt"),
		nats.UserCredentials(auth.InternalCredsPath()),
	)
	if err != nil {
		ns.Shutdown()
		ns.WaitForShutdown()
		return nil, fmt.Errorf("natsbus: in-process connect: %w", err)
	}

	if err := declareStreams(mgmt, cfg.Hub); err != nil {
		mgmt.Close()
		ns.Shutdown()
		ns.WaitForShutdown()
		return nil, err
	}

	return &Server{
		ns:       ns,
		mgmtConn: mgmt,
		storeDir: storeDir,
		url:      ns.ClientURL(),
		auth:     auth,
	}, nil
}

// NATSServer returns the underlying embedded server so in-process
// clients can pass it to nats.InProcessServer.
func (s *Server) NATSServer() *server.Server { return s.ns }

// ClientURL returns the nats:// URL the embedded server is listening on.
func (s *Server) ClientURL() string { return s.url }

// StoreDir returns the JetStream storage directory.
func (s *Server) StoreDir() string { return s.storeDir }

// Auth returns the store of operator/account/user NKey material. Used
// by the admin flow to mint and rotate per-source credentials.
func (s *Server) Auth() *AuthStore { return s.auth }

// ConnectedClientIPs returns the public IPs of the open NATS connections
// authenticated as userPubKey, most recently active first. The directory
// gate and the hub's UDP poller use it to find a collector without DNS,
// so a new IP counts as soon as the collector reconnects.
//
// Connz lists JWT-authenticated clients by user NKey, so this takes the
// pubkey (sources.user_pubkey) rather than a source ID. During a
// reconnect the old session lingers until its keepalive times out; the
// poller takes index 0, the newest, and the gate admits them all.
//
// It returns nil for an empty userPubKey or when no connection is open.
// IPv4-mapped IPv6 addresses are unmapped so callers can compare them
// with UDP source addresses.
func (s *Server) ConnectedClientIPs(userPubKey string) []netip.Addr {
	if s == nil || s.ns == nil || userPubKey == "" {
		return nil
	}
	cz, err := s.ns.Connz(&server.ConnzOptions{
		Username: true,
		User:     userPubKey,
		State:    server.ConnOpen,
		Sort:     server.ByLast,
	})
	if err != nil || cz == nil {
		return nil
	}
	out := make([]netip.Addr, 0, len(cz.Conns))
	seen := make(map[netip.Addr]struct{}, len(cz.Conns))
	for _, c := range cz.Conns {
		ip, err := netip.ParseAddr(c.IP)
		if err != nil {
			continue
		}
		ip = ip.Unmap()
		if _, dup := seen[ip]; dup {
			continue
		}
		seen[ip] = struct{}{}
		out = append(out, ip)
	}
	return out
}

// ConnectInternal opens an in-process NATS client authenticated as the
// hub-internal user (full pub/sub under TRINITY). Extra nats options,
// such as nats.Name, follow the required auth options.
func (s *Server) ConnectInternal(extra ...nats.Option) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.InProcessServer(s.ns),
		nats.UserCredentials(s.auth.InternalCredsPath()),
	}
	opts = append(opts, extra...)
	return nats.Connect("", opts...)
}

// Stop shuts down the embedded server and waits for it to drain.
func (s *Server) Stop() {
	if s == nil {
		return
	}
	if s.mgmtConn != nil {
		s.mgmtConn.Close()
		s.mgmtConn = nil
	}
	if s.ns != nil {
		s.ns.Shutdown()
		s.ns.WaitForShutdown()
	}
}

func declareStreams(nc *nats.Conn, hub *config.HubConfig) error {
	js, err := nc.JetStream()
	if err != nil {
		return fmt.Errorf("natsbus: JetStream context: %w", err)
	}

	if err := upsertStream(js, &nats.StreamConfig{
		Name:       StreamEvents,
		Subjects:   []string{"trinity.events.>"},
		Retention:  nats.LimitsPolicy,
		MaxAge:     hub.Retention.D(),
		MaxBytes:   EventsMaxBytes,
		Storage:    nats.FileStorage,
		Duplicates: hub.DedupWindow.D(),
	}); err != nil {
		return err
	}

	// The register stream keeps the latest payload per source, so a hub
	// that starts late sees the current roster even after the publisher
	// disconnects.
	if err := upsertStream(js, &nats.StreamConfig{
		Name:              StreamRegister,
		Subjects:          []string{"trinity.register.>"},
		Retention:         nats.LimitsPolicy,
		MaxMsgsPerSubject: 1,
		Storage:           nats.FileStorage,
	}); err != nil {
		return err
	}

	return nil
}

func upsertStream(js nats.JetStreamContext, cfg *nats.StreamConfig) error {
	if _, err := js.StreamInfo(cfg.Name); err == nil {
		if _, err := js.UpdateStream(cfg); err != nil {
			return fmt.Errorf("natsbus: UpdateStream %s: %w", cfg.Name, err)
		}
		return nil
	}
	if _, err := js.AddStream(cfg); err != nil {
		return fmt.Errorf("natsbus: AddStream %s: %w", cfg.Name, err)
	}
	return nil
}

func parseBindAddr(raw string) (string, int, error) {
	if raw == "" {
		return "", 0, fmt.Errorf("natsbus: tracker.nats.url is required for hub mode")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", 0, fmt.Errorf("natsbus: invalid tracker.nats.url %q: %v", raw, err)
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		return "", 0, fmt.Errorf("natsbus: invalid host:port in %q: %w", raw, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("natsbus: invalid port in %q: %w", raw, err)
	}
	return host, port, nil
}
