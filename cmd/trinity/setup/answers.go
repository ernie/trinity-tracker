// Package setup is the interactive installer ("trinity init" wizard)
// and the shared per-server prompt machinery used by `trinity server
// add`. It owns the embedded systemd units and per-gametype cfg
// templates that get rendered into a fresh install.
package setup

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ernie/trinity-tracker/internal/config"
)

// Mode picks which Trinity role(s) the install runs.
type Mode int

const (
	ModeCombined  Mode = iota // hub and local collector on one machine
	ModeHubOnly               // central UI; receives events from remote collectors
	ModeCollector             // tails local game server logs, reports to a remote hub
)

// String returns the operator-facing name of the mode.
func (m Mode) String() string {
	switch m {
	case ModeCombined:
		return "Hub + local collector"
	case ModeHubOnly:
		return "Hub only"
	case ModeCollector:
		return "Collector only"
	default:
		return fmt.Sprintf("mode(%d)", m)
	}
}

// Answers holds everything the wizard collected from the operator. The
// pure functions in this file convert it to a *config.Config and to
// the side files (per-server .env, gametype .cfg) Apply writes.
type Answers struct {
	Mode Mode

	// Common (all modes)
	ServiceUser string // service user, default "quake"
	ListenAddr  string // server.listen_addr
	HTTPPort    int    // server.http_port

	// Hub modes
	DatabasePath string // database.path
	StaticDir    string // server.static_dir (web assets root)
	JWTSecret    string // auth.jwt_secret; empty for collector-only

	// Hub modes (optional Discord weekly digest). Apply installs
	// trinity-digest.{service,timer} when DiscordEnabled is true.
	DiscordEnabled    bool
	DiscordWebhookURL string // full https://discord.com/api/webhooks/{id}/{token}
	DiscordSchedule   string // systemd OnCalendar= value, e.g. "Mon 00:00"

	// Collector modes
	InstallEngine bool   // download the latest trinity-engine release into Quake3Dir
	Quake3Dir     string // server.quake3_dir

	// Public URL and Let's Encrypt (collectors, and hub modes that serve
	// the SPA on a public hostname).
	HubHost    string // tracker.collector.hub_host (collector: remote hub; combined: this host)
	PublicURL  string // tracker.collector.public_url
	AdminEmail string // address Let's Encrypt uses for renewal notices
	SourceID   string // tracker.collector.source_id (combined: defaults to "hub")

	// Hub modes
	RemoteCollectorsExpected bool // bind NATS on 0.0.0.0 + TLS so remote collectors can connect

	// Collector-only
	CredsFile string // path to the operator's .creds file

	// Expert-mode skips, for operators who manage that piece themselves:
	//
	// SkipCert      installs nginx and its config but skips certbot, for
	//               a host with /etc/letsencrypt/live/ already staged
	// SkipFirewall  leaves ufw and firewalld alone
	// SkipNginx     skips nginx entirely, for a host with its own reverse
	//               proxy; implies SkipCert
	// SkipLogrotate skips /etc/logrotate.d/quake3
	SkipCert      bool
	SkipFirewall  bool
	SkipNginx     bool
	SkipLogrotate bool

	// Servers (collector and combined)
	Servers []ServerAnswers
}

// ServerAnswers is one game server entry, from the wizard's server loop
// or `trinity server add`.
type ServerAnswers struct {
	Key               string   // q3_servers[].key: lowercase letters, digits, _ and -
	Gametype          Gametype // the game's g_gametype value
	UseMissionpack    bool     // operator picked the Team Arena menu entry
	Address           string   // q3_servers[].address ("host:port")
	Port              int      // bind port (also embedded in address if collector)
	RconPassword      string   // q3_servers[].rcon_password
	LogPath           string   // q3_servers[].log_path
	AllowHubAdminRcon bool     // q3_servers[].allow_hub_admin_rcon
}

// RunsMissionpack reports whether the server starts with +set fs_game
// missionpack: Team Arena game types always do, TDM and CTF when picked.
func (s ServerAnswers) RunsMissionpack() bool {
	return s.UseMissionpack || s.Gametype.IsTeamArenaOnly()
}

func (s ServerAnswers) ModFolder() string {
	if s.RunsMissionpack() {
		return "missionpack"
	}
	return "baseq3"
}

// HasHubFields returns whether the chosen mode runs the hub role
// (and thus needs database/static-dir).
func (a *Answers) HasHubFields() bool {
	return a.Mode == ModeCombined || a.Mode == ModeHubOnly
}

// HasCollectorFields returns whether the chosen mode runs the
// collector role.
func (a *Answers) HasCollectorFields() bool {
	return a.Mode == ModeCombined || a.Mode == ModeCollector
}

// RunsLocalServers returns whether this install hosts game servers
// itself.
func (a *Answers) RunsLocalServers() bool {
	return a.HasCollectorFields()
}

// Validate runs cheap structural checks before Apply touches
// anything. Detailed config validation runs separately via
// config.validateTracker after the *config.Config has been built.
func (a *Answers) Validate() error {
	if a.ServiceUser == "" {
		return fmt.Errorf("service user is required")
	}
	if a.ListenAddr == "" {
		return fmt.Errorf("listen address is required")
	}
	if a.HTTPPort <= 0 || a.HTTPPort > 65535 {
		return fmt.Errorf("http port %d is out of range", a.HTTPPort)
	}
	if a.HasHubFields() {
		if a.DatabasePath == "" {
			return fmt.Errorf("database path is required for hub mode")
		}
		if a.StaticDir == "" {
			return fmt.Errorf("static dir is required for hub mode")
		}
		if a.PublicURL == "" {
			return fmt.Errorf("public URL is required for hub mode (used for nginx vhost + tracker.collector.public_url)")
		}
		if u, err := url.Parse(a.PublicURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			return fmt.Errorf("public URL %q must be an http(s) URL with a hostname", a.PublicURL)
		}
		// AdminEmail only matters to certbot.
		if a.AdminEmail == "" && !a.SkipCert && !a.SkipNginx {
			return fmt.Errorf("admin email is required for hub mode (Let's Encrypt renewal notices)")
		}
		if a.DiscordEnabled {
			if a.DiscordWebhookURL == "" {
				return fmt.Errorf("discord webhook URL is required when discord digest is enabled")
			}
			if a.DiscordSchedule == "" {
				return fmt.Errorf("discord schedule is required when discord digest is enabled")
			}
		}
	}
	if a.Mode == ModeCombined && a.SourceID == "" {
		return fmt.Errorf("source ID is required for combined mode (the local collector half publishes events under this name)")
	}
	if a.RunsLocalServers() && a.Quake3Dir == "" {
		return fmt.Errorf("quake3 dir is required when running local servers")
	}
	if a.Mode == ModeCollector {
		if a.HubHost == "" {
			return fmt.Errorf("hub host is required for collector-only mode")
		}
		if a.PublicURL == "" {
			return fmt.Errorf("public URL is required for collector-only mode")
		}
		if u, err := url.Parse(a.PublicURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			return fmt.Errorf("public URL %q must be an http(s) URL with a hostname", a.PublicURL)
		}
		if a.AdminEmail == "" {
			return fmt.Errorf("admin email is required for collector-only mode (Let's Encrypt renewal notices)")
		}
		if a.SourceID == "" {
			return fmt.Errorf("source ID is required for collector-only mode")
		}
		if a.CredsFile == "" {
			return fmt.Errorf("creds file is required for collector-only mode")
		}
	}
	seen := make(map[string]bool)
	seenPort := make(map[int]bool)
	for i, s := range a.Servers {
		if s.Key == "" {
			return fmt.Errorf("servers[%d]: key is required", i)
		}
		if !validKey(s.Key) {
			return fmt.Errorf("servers[%d].key %q must be alnum/underscore/hyphen, max 64 chars", i, s.Key)
		}
		lk := strings.ToLower(s.Key)
		if seen[lk] {
			return fmt.Errorf("servers[%d]: duplicate key %q", i, s.Key)
		}
		seen[lk] = true
		if s.Port <= 0 || s.Port > 65535 {
			return fmt.Errorf("servers[%d].port %d is out of range", i, s.Port)
		}
		if seenPort[s.Port] {
			return fmt.Errorf("servers[%d]: duplicate port %d", i, s.Port)
		}
		seenPort[s.Port] = true
		if s.Address == "" {
			return fmt.Errorf("servers[%d].address is required", i)
		}
		if s.LogPath == "" {
			return fmt.Errorf("servers[%d].log_path is required", i)
		}
		if s.RconPassword == "" {
			return fmt.Errorf("servers[%d].rcon_password is required", i)
		}
	}
	return nil
}

// Every collector ships with live spectating on: the relay binds this
// loopback address, and viewers watch this far behind real time. Both go
// into the generated config so operators can see and change them.
const (
	defaultLiveRelayAddr = "127.0.0.1:8081"
	defaultLiveDelay     = 5 * time.Second
)

// ToConfig builds a *config.Config from the wizard's answers. It does
// not write anything; the caller serializes via config.Save and runs
// config.Load round-trip validation.
func (a *Answers) ToConfig() *config.Config {
	useSd := true
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddr:  a.ListenAddr,
			HTTPPort:    a.HTTPPort,
			Quake3Dir:   a.Quake3Dir,
			ServiceUser: a.ServiceUser,
			UseSystemd:  &useSd,
		},
	}
	if a.HasHubFields() {
		cfg.Database = &config.DatabaseConfig{Path: a.DatabasePath}
		// The JWT secret is generated once and kept in config.yml so
		// reruns keep existing tokens valid; an empty secret would make
		// tokens forgeable. TokenDuration stays zero: users.token_version
		// governs session lifetime.
		cfg.Auth = &config.AuthConfig{JWTSecret: a.JWTSecret}
		if a.DiscordEnabled {
			// Empty categories use the digest's defaults.
			cfg.Discord = &config.DiscordConfig{
				Digest: &config.DigestConfig{WebhookURL: a.DiscordWebhookURL},
			}
		}
	}
	// Collectors use static_dir too: levelshots and demobake write there
	// for nginx to serve to the hub's web engine.
	if a.StaticDir != "" {
		cfg.Server.StaticDir = a.StaticDir
	}

	for _, s := range a.Servers {
		cfg.Q3Servers = append(cfg.Q3Servers, config.Q3Server{
			Key:               s.Key,
			Address:           s.Address,
			LogPath:           s.LogPath,
			RconPassword:      s.RconPassword,
			AllowHubAdminRcon: s.AllowHubAdminRcon,
		})
	}

	// Hub modes put the hub's PublicURL in tracker.collector.public_url
	// so URLs embedded in events resolve, and bind NATS externally only
	// when remote collectors are expected.
	switch a.Mode {
	case ModeCombined:
		cfg.Tracker = &config.TrackerConfig{
			Hub: &config.HubConfig{},
			Collector: &config.CollectorConfig{
				SourceID:      a.SourceID,
				DataDir:       "/var/lib/trinity",
				PublicURL:     a.PublicURL,
				HubHost:       HostFromURL(a.PublicURL),
				LiveRelayAddr: defaultLiveRelayAddr,
				LiveDelay:     config.Duration(defaultLiveDelay),
			},
			NATS: hubNATSConfig(a.RemoteCollectorsExpected),
		}
	case ModeHubOnly:
		cfg.Tracker = &config.TrackerConfig{
			Hub:  &config.HubConfig{},
			NATS: hubNATSConfig(a.RemoteCollectorsExpected),
		}
	case ModeCollector:
		cfg.Tracker = &config.TrackerConfig{
			Collector: &config.CollectorConfig{
				SourceID:      a.SourceID,
				DataDir:       "/var/lib/trinity",
				PublicURL:     a.PublicURL,
				HubHost:       a.HubHost,
				LiveRelayAddr: defaultLiveRelayAddr,
				LiveDelay:     config.Duration(defaultLiveDelay),
			},
			NATS: config.NATSConfig{
				CredentialsFile: "/etc/trinity/source.creds",
			},
		}
	}
	return cfg
}

// hubNATSConfig returns the embedded-NATS config for hub modes:
// localhost-only by default; externally bound with TLS when remote
// collectors are expected. The TLS files are symlinks created by Apply
// pointing at /etc/letsencrypt/live/<host>/.
func hubNATSConfig(remoteCollectorsExpected bool) config.NATSConfig {
	if !remoteCollectorsExpected {
		return config.NATSConfig{URL: "nats://127.0.0.1:4222"}
	}
	return config.NATSConfig{
		URL:      "nats://0.0.0.0:4222",
		CertFile: "/etc/trinity/tls/fullchain.pem",
		KeyFile:  "/etc/trinity/tls/privkey.pem",
	}
}

// validKey matches config.Load's rule for q3_servers[].key: lowercase
// letters, digits, _ and -, up to 64 characters. Keys are lowercase
// because the cfg and rotation filenames on disk are.
func validKey(key string) bool {
	if key == "" || len(key) > 64 {
		return false
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}
