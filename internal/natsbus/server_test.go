package natsbus

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/config"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func startTestServer(t *testing.T) *Server {
	t.Helper()
	port := freePort(t)
	cfg := &config.TrackerConfig{
		NATS: config.NATSConfig{URL: fmt.Sprintf("nats://127.0.0.1:%d", port)},
		Hub: &config.HubConfig{
			DedupWindow: config.Duration(30 * time.Minute),
			Retention:   config.Duration(10 * 24 * time.Hour),
		},
	}
	s, err := Start(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(s.Stop)
	return s
}

func TestStartAndClientURLListens(t *testing.T) {
	s := startTestServer(t)

	nc, err := nats.Connect("", nats.InProcessServer(s.NATSServer()))
	if err != nil {
		t.Fatalf("InProcessServer connect: %v", err)
	}
	defer nc.Close()

	if !nc.IsConnected() {
		t.Fatal("expected connected NATS client")
	}
	if s.ClientURL() == "" {
		t.Error("ClientURL empty")
	}
}

func TestStreamsDeclared(t *testing.T) {
	s := startTestServer(t)

	nc, err := nats.Connect("", nats.InProcessServer(s.NATSServer()))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}

	events, err := js.StreamInfo(StreamEvents)
	if err != nil {
		t.Fatalf("StreamInfo(%s): %v", StreamEvents, err)
	}
	if events.Config.Retention != nats.LimitsPolicy {
		t.Errorf("events retention = %v, want LimitsPolicy", events.Config.Retention)
	}
	if events.Config.MaxAge != 10*24*time.Hour {
		t.Errorf("events MaxAge = %v, want 10d", events.Config.MaxAge)
	}
	if events.Config.Duplicates != 30*time.Minute {
		t.Errorf("events Duplicates = %v, want 30m", events.Config.Duplicates)
	}
	if events.Config.MaxBytes != EventsMaxBytes {
		t.Errorf("events MaxBytes = %d, want %d", events.Config.MaxBytes, EventsMaxBytes)
	}
	if events.Config.Storage != nats.FileStorage {
		t.Errorf("events Storage = %v, want FileStorage", events.Config.Storage)
	}

	reg, err := js.StreamInfo(StreamRegister)
	if err != nil {
		t.Fatalf("StreamInfo(%s): %v", StreamRegister, err)
	}
	if reg.Config.MaxMsgsPerSubject != 1 {
		t.Errorf("register MaxMsgsPerSubject = %d, want 1", reg.Config.MaxMsgsPerSubject)
	}
	if reg.Config.Storage != nats.FileStorage {
		t.Errorf("register Storage = %v, want FileStorage", reg.Config.Storage)
	}
}

func TestStartRequiresHub(t *testing.T) {
	if _, err := Start(nil, t.TempDir()); err == nil {
		t.Error("expected error for nil tracker")
	}
	if _, err := Start(&config.TrackerConfig{}, t.TempDir()); err == nil {
		t.Error("expected error when hub sub-config missing")
	}
}

func TestStartIsIdempotentForReboot(t *testing.T) {
	port := freePort(t)
	storeParent := t.TempDir()
	cfg := &config.TrackerConfig{
		NATS: config.NATSConfig{URL: fmt.Sprintf("nats://127.0.0.1:%d", port)},
		Hub:  &config.HubConfig{DedupWindow: config.Duration(time.Minute), Retention: config.Duration(time.Hour)},
	}

	s1, err := Start(cfg, storeParent)
	if err != nil {
		t.Fatalf("first Start: %v", err)
	}
	s1.Stop()

	s2, err := Start(cfg, storeParent)
	if err != nil {
		t.Fatalf("second Start (reusing store): %v", err)
	}
	defer s2.Stop()

	nc, err := nats.Connect("", nats.InProcessServer(s2.NATSServer()))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	if _, err := js.StreamInfo(StreamEvents); err != nil {
		t.Errorf("events stream missing after reboot: %v", err)
	}
}
