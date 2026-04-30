package setup

import (
	"strings"
	"testing"

	"github.com/ernie/trinity-tracker/internal/config"
)

func validCombinedAnswers() *Answers {
	return &Answers{
		Mode:         ModeCombined,
		ServiceUser:  "quake",
		ListenAddr:   "127.0.0.1",
		HTTPPort:     8080,
		DatabasePath: "/var/lib/trinity/trinity.db",
		StaticDir:    "/var/lib/trinity/web",
		Quake3Dir:    "/usr/lib/quake3",
		PublicURL:    "https://hub.example.com",
		AdminEmail:   "ops@example.com",
		SourceID:     "hub",
		Servers: []ServerAnswers{
			{Key: "ffa", Gametype: GametypeFFA, Address: "127.0.0.1:27960", Port: 27960, RconPassword: "secret", LogPath: "/var/log/quake3/ffa.log"},
		},
	}
}

func validHubOnlyAnswers() *Answers {
	return &Answers{
		Mode:         ModeHubOnly,
		ServiceUser:  "quake",
		ListenAddr:   "0.0.0.0",
		HTTPPort:     8080,
		DatabasePath: "/var/lib/trinity/trinity.db",
		StaticDir:    "/var/lib/trinity/web",
		PublicURL:    "https://hub.example.com",
		AdminEmail:   "ops@example.com",
	}
}

func TestAnswersValidate_Combined_Valid(t *testing.T) {
	if err := validCombinedAnswers().Validate(); err != nil {
		t.Fatalf("expected valid combined answers, got %v", err)
	}
}

func TestAnswersValidate_HubOnly_Valid(t *testing.T) {
	if err := validHubOnlyAnswers().Validate(); err != nil {
		t.Fatalf("hub-only valid: %v", err)
	}
}

func TestAnswersValidate_CollectorOnly_Valid(t *testing.T) {
	a := &Answers{
		Mode:        ModeCollector,
		ServiceUser: "quake",
		ListenAddr:  "127.0.0.1",
		HTTPPort:    8080,
		Quake3Dir:   "/usr/lib/quake3",
		HubHost:     "trinity.run",
		PublicURL:   "https://q3.example.com",
		AdminEmail:  "ops@example.com",
		SourceID:    "mygame",
		CredsFile:   "/tmp/mygame.creds",
		Servers: []ServerAnswers{
			{Key: "ffa", Gametype: GametypeFFA, Address: "q3.example.com:27960", Port: 27960, RconPassword: "secret", LogPath: "/var/log/quake3/ffa.log"},
		},
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("collector-only valid: %v", err)
	}
}

func TestAnswersValidate_Errors(t *testing.T) {
	cases := []struct {
		name    string
		mut     func(*Answers)
		want    string
	}{
		{"missing service user", func(a *Answers) { a.ServiceUser = "" }, "service user"},
		{"bad port", func(a *Answers) { a.HTTPPort = 70000 }, "out of range"},
		{"missing db path in hub mode", func(a *Answers) { a.DatabasePath = "" }, "database path"},
		{"missing static dir in hub mode", func(a *Answers) { a.StaticDir = "" }, "static dir"},
		{"bad server key chars", func(a *Answers) { a.Servers[0].Key = "ffa!" }, "alnum"},
		{"duplicate keys", func(a *Answers) {
			a.Servers = append(a.Servers, ServerAnswers{
				Key: "ffa", Gametype: GametypeFFA, Address: "127.0.0.1:27961", Port: 27961, RconPassword: "x", LogPath: "/var/log/quake3/2.log",
			})
		}, "duplicate key"},
		{"duplicate ports", func(a *Answers) {
			a.Servers = append(a.Servers, ServerAnswers{
				Key: "1v1", Gametype: GametypeTournament, Address: "127.0.0.1:27960", Port: 27960, RconPassword: "x", LogPath: "/var/log/quake3/1v1.log",
			})
		}, "duplicate port"},
		{"empty rcon", func(a *Answers) { a.Servers[0].RconPassword = "" }, "rcon_password"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := validCombinedAnswers()
			tc.mut(a)
			err := a.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v does not contain %q", err, tc.want)
			}
		})
	}
}

func TestAnswersValidate_CollectorOnly_RequiresHubFields(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Answers)
		want string
	}{
		{"no hub host", func(a *Answers) { a.HubHost = "" }, "hub host"},
		{"no public url", func(a *Answers) { a.PublicURL = "" }, "public URL"},
		{"bad public url", func(a *Answers) { a.PublicURL = "ftp://x.example.com" }, "http(s) URL"},
		{"no admin email", func(a *Answers) { a.AdminEmail = "" }, "admin email"},
		{"no source id", func(a *Answers) { a.SourceID = "" }, "source ID"},
		{"no creds file", func(a *Answers) { a.CredsFile = "" }, "creds file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &Answers{
				Mode:        ModeCollector,
				ServiceUser: "quake",
				ListenAddr:  "127.0.0.1",
				HTTPPort:    8080,
				Quake3Dir:   "/usr/lib/quake3",
				HubHost:     "trinity.run",
				PublicURL:   "https://q3.example.com",
				AdminEmail:  "ops@example.com",
				SourceID:    "mygame",
				CredsFile:   "/tmp/mygame.creds",
			}
			tc.mut(a)
			err := a.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v does not contain %q", err, tc.want)
			}
		})
	}
}

func TestToConfig_Combined_EmitsHubAndCollectorBlocks(t *testing.T) {
	a := validCombinedAnswers()
	cfg := a.ToConfig()
	if cfg.Tracker == nil {
		t.Fatalf("expected explicit tracker block for combined mode")
	}
	if cfg.Tracker.Hub == nil {
		t.Errorf("combined should emit hub block")
	}
	if cfg.Tracker.Collector == nil {
		t.Fatalf("combined should emit collector block, got %+v", cfg.Tracker)
	}
	if cfg.Tracker.Collector.SourceID != "hub" {
		t.Errorf("source id: got %q want %q", cfg.Tracker.Collector.SourceID, "hub")
	}
	if cfg.Tracker.Collector.PublicURL != "https://hub.example.com" {
		t.Errorf("public url: got %q", cfg.Tracker.Collector.PublicURL)
	}
	if cfg.Tracker.Collector.HubHost != "hub.example.com" {
		t.Errorf("hub host: got %q (should be PublicURL hostname)", cfg.Tracker.Collector.HubHost)
	}
	// Default (no remote collectors) → localhost-only NATS, no TLS.
	if cfg.Tracker.NATS.URL != "nats://127.0.0.1:4222" {
		t.Errorf("nats url: got %q", cfg.Tracker.NATS.URL)
	}
	if cfg.Tracker.NATS.CertFile != "" || cfg.Tracker.NATS.KeyFile != "" {
		t.Errorf("nats TLS should be empty for local-only hub: %+v", cfg.Tracker.NATS)
	}
	if cfg.Database.Path != "/var/lib/trinity/trinity.db" {
		t.Errorf("db path: got %q", cfg.Database.Path)
	}
	if len(cfg.Q3Servers) != 1 || cfg.Q3Servers[0].Key != "ffa" {
		t.Errorf("server entries: %+v", cfg.Q3Servers)
	}
}

func TestToConfig_Combined_RemoteCollectors_BindsExternalNATS(t *testing.T) {
	a := validCombinedAnswers()
	a.RemoteCollectorsExpected = true
	cfg := a.ToConfig()
	if cfg.Tracker.NATS.URL != "nats://0.0.0.0:4222" {
		t.Errorf("nats url: got %q want external bind", cfg.Tracker.NATS.URL)
	}
	if cfg.Tracker.NATS.CertFile != "/etc/trinity/tls/fullchain.pem" {
		t.Errorf("nats cert_file: got %q", cfg.Tracker.NATS.CertFile)
	}
	if cfg.Tracker.NATS.KeyFile != "/etc/trinity/tls/privkey.pem" {
		t.Errorf("nats key_file: got %q", cfg.Tracker.NATS.KeyFile)
	}
}

func TestToConfig_HubOnly_NoCollectorBlock(t *testing.T) {
	a := validHubOnlyAnswers()
	cfg := a.ToConfig()
	if cfg.Tracker == nil || cfg.Tracker.Hub == nil {
		t.Fatalf("expected hub block, got %+v", cfg.Tracker)
	}
	if cfg.Tracker.Collector != nil {
		t.Errorf("hub-only should not have collector block: %+v", cfg.Tracker.Collector)
	}
	// Default (no remote collectors) → localhost-only NATS.
	if cfg.Tracker.NATS.URL != "nats://127.0.0.1:4222" {
		t.Errorf("hub-only nats url: got %q", cfg.Tracker.NATS.URL)
	}
}

func TestToConfig_HubOnly_RemoteCollectors_BindsExternalNATS(t *testing.T) {
	a := validHubOnlyAnswers()
	a.RemoteCollectorsExpected = true
	cfg := a.ToConfig()
	if cfg.Tracker.NATS.URL != "nats://0.0.0.0:4222" {
		t.Errorf("nats url: got %q want external bind", cfg.Tracker.NATS.URL)
	}
	if cfg.Tracker.NATS.CertFile == "" || cfg.Tracker.NATS.KeyFile == "" {
		t.Errorf("expected TLS cert + key paths, got %+v", cfg.Tracker.NATS)
	}
}

func TestToConfig_CollectorOnly_NoHubBlock(t *testing.T) {
	a := &Answers{
		Mode:        ModeCollector,
		ServiceUser: "quake",
		ListenAddr:  "127.0.0.1",
		HTTPPort:    8080,
		Quake3Dir:   "/usr/lib/quake3",
		HubHost:     "trinity.run",
		PublicURL:   "https://q3.example.com",
		AdminEmail:  "ops@example.com",
		SourceID:    "mygame",
		CredsFile:   "/tmp/mygame.creds",
	}
	cfg := a.ToConfig()
	if cfg.Tracker == nil || cfg.Tracker.Collector == nil {
		t.Fatalf("expected collector block, got %+v", cfg.Tracker)
	}
	if cfg.Tracker.Hub != nil {
		t.Errorf("collector-only should not have hub block")
	}
	if cfg.Tracker.Collector.SourceID != "mygame" {
		t.Errorf("source id: got %q", cfg.Tracker.Collector.SourceID)
	}
	if cfg.Tracker.NATS.CredentialsFile != "/etc/trinity/source.creds" {
		t.Errorf("creds file path: got %q", cfg.Tracker.NATS.CredentialsFile)
	}
	if cfg.Database != nil {
		t.Errorf("collector-only should omit database block entirely, got %+v", cfg.Database)
	}
	if cfg.Auth != nil {
		t.Errorf("collector-only should omit auth block entirely, got %+v", cfg.Auth)
	}
}

func TestToConfig_RoundTripsThroughValidator(t *testing.T) {
	for _, mode := range []Mode{ModeCombined, ModeHubOnly, ModeCollector} {
		t.Run(mode.String(), func(t *testing.T) {
			var a *Answers
			switch mode {
			case ModeCombined:
				a = validCombinedAnswers()
			case ModeHubOnly:
				a = validHubOnlyAnswers()
			case ModeCollector:
				a = &Answers{
					Mode: ModeCollector, ServiceUser: "quake", ListenAddr: "127.0.0.1", HTTPPort: 8080,
					Quake3Dir: "/usr/lib/quake3", HubHost: "trinity.run",
					PublicURL: "https://q3.example.com", SourceID: "mygame", CredsFile: "/tmp/mygame.creds",
					Servers: []ServerAnswers{
						{Key: "ffa", Gametype: GametypeFFA, Address: "q3.example.com:27960", Port: 27960, RconPassword: "secret", LogPath: "/var/log/quake3/ffa.log"},
					},
				}
			}
			cfg := a.ToConfig()
			if err := config.ValidateForSave(cfg); err != nil {
				t.Fatalf("ValidateForSave for %s: %v", mode, err)
			}
		})
	}
}
