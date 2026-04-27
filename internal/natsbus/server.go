// Package natsbus owns the embedded NATS server used in hub mode and
// exposes helpers for declaring the JetStream streams that carry
// distributed-tracking traffic. Phase 2 of the M2 rollout: only the
// server + streams exist; publishers, subscribers, and RPC wiring land
// in later phases.
package natsbus

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/config"
)

// Stream names. Exported so phases 3+ can subscribe / publish by name.
const (
	StreamEvents   = "TRINITY_EVENTS"
	StreamRegister = "TRINITY_REGISTER"
)

// EventsMaxBytes is the byte-based side of the limits-policy retention
// for TRINITY_EVENTS, paired with the configured max age. 1 GiB — event
// envelopes are a few hundred bytes each, so at realistic rates this
// bound is far above the age-based limit and just guards disk use.
const EventsMaxBytes = 1 * 1024 * 1024 * 1024

// Server owns the embedded nats-server process and the in-process
// management connection used to declare streams.
type Server struct {
	ns       *server.Server
	mgmtConn *nats.Conn
	storeDir string
	url      string
}

// Start boots the embedded server, waits for it to accept connections,
// and declares the TRINITY_EVENTS + TRINITY_REGISTER streams. The
// returned Server is ready for in-process clients via nats.Connect with
// nats.InProcessServer(s.NATSServer()).
//
// storeDirParent is the directory under which JetStream's state is
// persisted (a "nats" subdirectory is created and used).
func Start(cfg *config.TrackerConfig, storeDirParent string) (*Server, error) {
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

	opts := &server.Options{
		Host:              host,
		Port:              port,
		JetStream:         true,
		StoreDir:          storeDir,
		JetStreamMaxStore: 2 * EventsMaxBytes,
		NoLog:             true,
		NoSigs:            true,
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

	mgmt, err := nats.Connect("", nats.InProcessServer(ns), nats.Name("trinity-hub-mgmt"))
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
	}, nil
}

// NATSServer returns the underlying embedded server so in-process
// clients can pass it to nats.InProcessServer.
func (s *Server) NATSServer() *server.Server { return s.ns }

// ClientURL returns the nats:// URL the embedded server is listening on.
func (s *Server) ClientURL() string { return s.url }

// StoreDir returns the JetStream storage directory.
func (s *Server) StoreDir() string { return s.storeDir }

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

	// Register stream keeps the latest payload per source so a late-
	// joining hub sees the current roster without needing the publisher
	// to still be connected. Retention semantics may be revisited in
	// phase 6 when the subscriber lands.
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
