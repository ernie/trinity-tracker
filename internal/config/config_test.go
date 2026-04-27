package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return p
}

func TestLoadStandaloneHasNilTracker(t *testing.T) {
	p := writeConfig(t, `
server:
  listen_addr: "127.0.0.1"
  http_port: 8080
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Tracker != nil {
		t.Fatalf("expected Tracker nil, got %+v", cfg.Tracker)
	}
}

func TestLoadTrackerHubOnlyAppliesDefaults(t *testing.T) {
	p := writeConfig(t, `
tracker:
  hub: {}
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Tracker == nil || cfg.Tracker.Hub == nil {
		t.Fatalf("expected Tracker.Hub populated")
	}
	if cfg.Tracker.Collector != nil {
		t.Fatalf("expected Tracker.Collector nil, got %+v", cfg.Tracker.Collector)
	}
	if got := cfg.Tracker.NATS.URL; got != "nats://localhost:4222" {
		t.Errorf("NATS.URL default = %q, want nats://localhost:4222", got)
	}
	if got := cfg.Tracker.Hub.DedupWindow.D(); got != 30*time.Minute {
		t.Errorf("DedupWindow default = %v, want 30m", got)
	}
	if got := cfg.Tracker.Hub.Retention.D(); got != 10*24*time.Hour {
		t.Errorf("Retention default = %v, want 10d", got)
	}
}

func TestLoadTrackerCollectorOnly(t *testing.T) {
	p := writeConfig(t, `
tracker:
  nats:
    url: "nats://hub.example.com:4222"
    credentials_file: "/etc/trinity/x.creds"
  collector:
    source_id: "chicago-ffa"
    data_dir: "/var/lib/trinity"
    demo_base_url: "https://example.com"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := cfg.Tracker.Collector
	if c == nil {
		t.Fatal("expected Collector set")
	}
	if c.SourceID != "chicago-ffa" {
		t.Errorf("SourceID = %q", c.SourceID)
	}
	if c.DataDir != "/var/lib/trinity" {
		t.Errorf("DataDir = %q", c.DataDir)
	}
	if got := c.HeartbeatInterval.D(); got != 30*time.Second {
		t.Errorf("HeartbeatInterval default = %v, want 30s", got)
	}
	if c.ClaimURL != "trinity.ernie.io" {
		t.Errorf("ClaimURL default = %q", c.ClaimURL)
	}
	if cfg.Tracker.NATS.CredentialsFile != "/etc/trinity/x.creds" {
		t.Errorf("CredentialsFile = %q", cfg.Tracker.NATS.CredentialsFile)
	}
	if cfg.Tracker.Hub != nil {
		t.Errorf("expected Hub nil")
	}
}

func TestLoadTrackerHubPlusCollector(t *testing.T) {
	p := writeConfig(t, `
tracker:
  hub:
    dedup_window: "15m"
    retention: "5d"
    approval_required: false
  collector:
    source_id: "local"
    data_dir: "/var/lib/trinity"
    heartbeat_interval: "10s"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Tracker.Hub == nil || cfg.Tracker.Collector == nil {
		t.Fatal("expected both Hub and Collector set")
	}
	if got := cfg.Tracker.Hub.DedupWindow.D(); got != 15*time.Minute {
		t.Errorf("DedupWindow = %v", got)
	}
	if got := cfg.Tracker.Hub.Retention.D(); got != 5*24*time.Hour {
		t.Errorf("Retention = %v, want 5d", got)
	}
	if cfg.Tracker.Hub.ApprovalRequired {
		t.Errorf("ApprovalRequired = true, want false")
	}
	if got := cfg.Tracker.Collector.HeartbeatInterval.D(); got != 10*time.Second {
		t.Errorf("HeartbeatInterval = %v", got)
	}
}

func TestLoadTrackerWithoutRolesFails(t *testing.T) {
	p := writeConfig(t, `
tracker:
  nats:
    url: "nats://localhost:4222"
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected validation error for tracker with no hub/collector")
	}
}

func TestLoadCollectorMissingSourceIDFails(t *testing.T) {
	p := writeConfig(t, `
tracker:
  collector:
    data_dir: "/var/lib/trinity"
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error when source_id missing")
	}
}

func TestLoadCollectorMissingDataDirFails(t *testing.T) {
	p := writeConfig(t, `
tracker:
  collector:
    source_id: "chicago-ffa"
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error when data_dir missing")
	}
}

func TestDurationParsesDaysSuffix(t *testing.T) {
	cases := map[string]time.Duration{
		"10d":  10 * 24 * time.Hour,
		"30m":  30 * time.Minute,
		"30s":  30 * time.Second,
		"2h":   2 * time.Hour,
		"500ms": 500 * time.Millisecond,
	}
	for in, want := range cases {
		got, err := parseDuration(in)
		if err != nil {
			t.Errorf("parseDuration(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseDuration(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := parseDuration("bogus"); err == nil {
		t.Error("expected error for bogus duration")
	}
	if _, err := parseDuration(""); err == nil {
		t.Error("expected error for empty duration")
	}
}
