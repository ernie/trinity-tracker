// Package setup is the interactive installer ("trinity init" wizard)
// and the shared per-server prompt machinery used by `trinity server
// add`. It owns the embedded systemd units and per-gametype cfg
// templates that get rendered into a fresh install.
package setup

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/ernie/trinity-tracker/internal/config"
)

// Mode picks which Trinity role(s) the install runs.
type Mode int

const (
	ModeCombined  Mode = iota // hub + local collector — single-machine default
	ModeHubOnly               // central UI; receives events from remote collectors
	ModeCollector             // tails local q3 logs, reports to a remote hub
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
	JWTSecret    string // auth.jwt_secret — generated once at install time
	// and persisted in /etc/trinity/config.yml. Empty for collector-only.

	// Hub modes (optional Discord weekly digest). Apply installs
	// trinity-digest.{service,timer} when DiscordEnabled is true.
	DiscordEnabled    bool
	DiscordWebhookURL string // full https://discord.com/api/webhooks/{id}/{token}
	DiscordSchedule   string // systemd OnCalendar= value, e.g. "Mon 00:00"

	// Collector modes
	InstallEngine bool   // download the latest trinity-engine release into Quake3Dir
	Quake3Dir     string // server.quake3_dir

	// Public-facing URL + LE (collector, plus hub modes that front the
	// SPA on a public hostname).
	HubHost    string // tracker.collector.hub_host (collector: remote hub; combined: this host)
	PublicURL  string // tracker.collector.public_url
	AdminEmail string // address Let's Encrypt uses for renewal notices
	SourceID   string // tracker.collector.source_id (combined: defaults to "hub")

	// Hub modes
	RemoteCollectorsExpected bool // bind NATS on 0.0.0.0 + TLS so remote collectors can connect

	// Collector-only
	CredsFile string // path to .creds file the operator received

	// Expert-mode skips. Each disables a slice of Apply's side effects
	// for operators who manage that piece of the stack themselves.
	//
	// SkipCert      install nginx + render config + reload, but don't run
	//               certbot. Caller has /etc/letsencrypt/live/ staged
	//               already (hub migration use case).
	// SkipFirewall  don't poke ufw/firewalld. Operator manages firewall
	//               via cloud dashboard, nftables, config-management, etc.
	// SkipNginx     don't install/configure nginx at all. Operator runs
	//               their own reverse proxy (Caddy, Traefik, manual nginx
	//               elsewhere, etc.). Implies SkipCert (no nginx → no
	//               certbot --nginx).
	// SkipLogrotate don't write /etc/logrotate.d/quake3. Operator manages
	//               log rotation via fluent-bit, vector, journald-only, etc.
	SkipCert      bool
	SkipFirewall  bool
	SkipNginx     bool
	SkipLogrotate bool

	// Servers (collector and combined)
	Servers []ServerAnswers
}

// ServerAnswers is one q3 server entry (collected per-server in the
// wizard's loop, also used by `trinity server add`).
type ServerAnswers struct {
	Key               string   // q3_servers[].key — short, alnum/underscore/hyphen
	Gametype          Gametype // q3 g_gametype value
	UseMissionpack    bool     // operator picked the (TA) menu variant
	Address           string   // q3_servers[].address ("host:port")
	Port              int      // bind port (also embedded in address if collector)
	RconPassword      string   // q3_servers[].rcon_password
	LogPath           string   // q3_servers[].log_path
	AllowHubAdminRcon bool     // q3_servers[].allow_hub_admin_rcon
}

// RunsMissionpack reports whether the server starts with +set fs_game
// missionpack (TA-only gametypes always do; TDM/CTF only when picked).
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

// RunsLocalServers returns whether this install hosts q3 servers
// itself (vs. a hub that just receives events from remote collectors).
// Today this is identical to HasCollectorFields, but kept distinct so
// hub-only installs that someday want to manage local q3 servers
// (e.g., for testing) don't conflate the two.
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
		// AdminEmail is for Let's Encrypt renewal alerts only — skip
		// the requirement when the operator has opted out of either
		// the certbot run or nginx altogether.
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
		// JWTSecret is install-time entropy: generated once by the
		// wizard and pinned in config.yml so re-applies don't
		// invalidate every existing auth token. Without it, the hub
		// signs JWTs with an empty secret — i.e. forgeable tokens.
		// TokenDuration is left zero on purpose; config.Load applies
		// the 24h default on round-trip.
		cfg.Auth = &config.AuthConfig{JWTSecret: a.JWTSecret}
		if a.DiscordEnabled {
			// DigestCategories left empty so the discord-digest
			// subcommand uses defaults; operators can edit the
			// config later to prune or reorder.
			cfg.Discord = &config.DiscordConfig{WebhookURL: a.DiscordWebhookURL}
		}
	}
	// static_dir is also useful for collectors: trinity levelshots and
	// trinity demobake write into {static_dir}/assets and {static_dir}/pk3s,
	// which nginx then serves to the hub's WASM engine. Default it for any
	// mode that has it set (the wizard always defaults a value).
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

	// Tracker block: emitted explicitly for every mode. Hub-bearing
	// modes carry the hub's PublicURL into tracker.collector.public_url
	// (so event-embedded URLs resolve correctly) and bind NATS externally
	// only when remote collectors are expected.
	switch a.Mode {
	case ModeCombined:
		cfg.Tracker = &config.TrackerConfig{
			Hub: &config.HubConfig{},
			Collector: &config.CollectorConfig{
				SourceID:  a.SourceID,
				DataDir:   "/var/lib/trinity",
				PublicURL: a.PublicURL,
				HubHost:   HostFromURL(a.PublicURL),
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
				SourceID:  a.SourceID,
				DataDir:   "/var/lib/trinity",
				PublicURL: a.PublicURL,
				HubHost:   a.HubHost,
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

// validKey is the same shape config.Load enforces for q3_servers[].key:
// lowercase alnum + underscore + hyphen, max 64 chars. Uppercase is
// rejected because filesystem paths and the cfg/rotation filenames are
// always lowercased — accepting mixed case would let the YAML and the
// hub identity diverge from what's on disk.
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
