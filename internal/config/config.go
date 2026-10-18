package config

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// idPattern validates q3_servers[].key. Keys are lowercase so the hub
// identity matches the always-lowercase file paths. It lives here so
// config needn't import storage.
var idPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

// Config holds the application configuration.
//
// Tracker is always set after Load: with no tracker block in the YAML,
// Load fills in a hub plus local collector.
type Config struct {
	Server ServerConfig `yaml:"server"`
	// Database and Auth are pointers so collector-only configs can
	// omit them. Load fills hub defaults; readers must nil-check.
	Database  *DatabaseConfig `yaml:"database,omitempty"`
	Auth      *AuthConfig     `yaml:"auth,omitempty"`
	Discord   *DiscordConfig  `yaml:"discord,omitempty"`
	Q3Servers []Q3Server      `yaml:"q3_servers,omitempty"`
	Tracker   *TrackerConfig  `yaml:"tracker,omitempty"`
}

// Duration extends time.Duration's YAML parsing to accept a "d" (days) suffix
// in addition to Go's stdlib units (ns, us/µs, ms, s, m, h).
type Duration time.Duration

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	parsed, err := parseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(parsed)
	return nil
}

// MarshalYAML emits "30s"/"24h" instead of yaml.v3's default raw
// nanosecond integer, which time.ParseDuration would later reject.
func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}

// D returns the wrapped time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if strings.HasSuffix(s, "d") {
		n, err := strconv.ParseInt(strings.TrimSuffix(s, "d"), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// TrackerConfig selects this process's roles by which of Hub and
// Collector are set:
//
//   - Both: hub plus local collector, the single-machine default.
//   - Hub only: a hub that accepts only remote collectors.
//   - Collector only: a remote collector feeding a hub over NATS.
type TrackerConfig struct {
	NATS      NATSConfig       `yaml:"nats"`
	Hub       *HubConfig       `yaml:"hub,omitempty"`
	Collector *CollectorConfig `yaml:"collector,omitempty"`
}

// NATSConfig points at the NATS endpoint. For a hub it is the bind address
// of the embedded server; for a remote collector it is the URL of the hub
// to connect to.
//
// CertFile and KeyFile, both set, enable TLS on the embedded server.
// A collector ignores them: it learns from the server's INFO message
// that TLS is required and upgrades, checking the server certificate
// against the system roots.
type NATSConfig struct {
	URL             string `yaml:"url"`
	CredentialsFile string `yaml:"credentials_file,omitempty"`
	CertFile        string `yaml:"cert_file,omitempty"`
	KeyFile         string `yaml:"key_file,omitempty"`
}

// HubConfig configures the aggregator role.
type HubConfig struct {
	DedupWindow Duration         `yaml:"dedup_window"`
	Retention   Duration         `yaml:"retention"`
	Directory   *DirectoryConfig `yaml:"directory,omitempty"`
}

// DirectoryConfig configures the optional Quake III directory (master)
// server. The hub binds UDP only when Enabled is true. It admits a
// heartbeat only from a server in the servers table whose stored
// address resolves to the heartbeat's source IP:port.
type DirectoryConfig struct {
	Enabled          bool     `yaml:"enabled"`
	ListenAddr       string   `yaml:"listen_addr,omitempty"`
	Port             int      `yaml:"port,omitempty"`
	HeartbeatExpiry  Duration `yaml:"heartbeat_expiry,omitempty"`
	ChallengeTimeout Duration `yaml:"challenge_timeout,omitempty"`
	GateRefresh      Duration `yaml:"gate_refresh,omitempty"`
	MaxServers       int      `yaml:"max_servers,omitempty"`
	// PersistedFreshness is how old the saved registry's newest
	// validated_at can be for it to be restored at startup; an older
	// snapshot is discarded. Raise it if deploys take longer than the
	// default 5 minutes.
	PersistedFreshness Duration `yaml:"persisted_freshness,omitempty"`
}

// CollectorConfig configures the collector role.
//
// PublicURL (required) is the collector host's public URL, such as
// https://q3.example.com. The hub uses it as the source's base URL for
// demos and other assets.
//
// HubHost (required) is the bare hostname of the hub; in collector-only
// mode it gives the default nats.url, nats://<hub_host>:4222.
//
// Each q3_servers[].address should be the server's public host:port
// (PublicURL's hostname plus the server's net_port). The collector
// sends rcon to it and the hub polls it over UDP.
type CollectorConfig struct {
	SourceID          string   `yaml:"source_id"`
	DataDir           string   `yaml:"data_dir"`
	HeartbeatInterval Duration `yaml:"heartbeat_interval"`
	PublicURL         string   `yaml:"public_url"`
	HubHost           string   `yaml:"hub_host"`

	// LiveRelayAddr is where the live relay listens, such as
	// "127.0.0.1:8081". Empty disables the relay.
	LiveRelayAddr string `yaml:"live_relay_addr,omitempty"`
	// LiveDelay is how far behind the game live viewers should be,
	// default 5s. A value at or below the keyframe interval
	// (sv_tvLiveKeyframeMsec) gives a delay of about one interval.
	LiveDelay Duration `yaml:"live_delay,omitempty"`
}

// AuthConfig holds authentication settings
type AuthConfig struct {
	JWTSecret     string        `yaml:"jwt_secret"`
	TokenDuration time.Duration `yaml:"token_duration"`
}

// DiscordConfig groups the optional Discord integrations; either block
// works alone. A webhook URL is a credential by itself, so the file
// should stay 0640.
//
// `trinity discord-digest`, run by the systemd timer, reads Digest.
// `trinity serve` reads Activity for the notifier that posts when a
// server's human count goes from zero to one or back.
type DiscordConfig struct {
	Digest   *DigestConfig   `yaml:"digest,omitempty"`
	Activity *ActivityConfig `yaml:"activity,omitempty"`

	// Old key names, read only so the validator can point at their
	// replacements; YAML ignores unknown keys, so an old config would
	// otherwise stop posting with no error.
	LegacyWebhookURL       string   `yaml:"webhook_url,omitempty"`
	LegacyDigestCategories []string `yaml:"digest_categories,omitempty"`
}

// DigestConfig drives the leaderboard digest.
//
// WebhookURL is the full https://discord.com/api/webhooks/{id}/{token}
// URL. Categories optionally replaces the default categories; each must
// be one /api/stats/leaderboard accepts.
type DigestConfig struct {
	WebhookURL string   `yaml:"webhook_url"`
	Categories []string `yaml:"categories,omitempty"`
}

// ActivityConfig drives the server activity notifier.
//
// ActiveDelaySeconds is the wait between the first human joining and
// the "active" post; InactiveDebounceSeconds is how long a server must
// stay empty before the "idle" post. A player who leaves and returns
// within either window produces no message.
type ActivityConfig struct {
	WebhookURL              string `yaml:"webhook_url"`
	ActiveDelaySeconds      int    `yaml:"active_delay_seconds,omitempty"`
	InactiveDebounceSeconds int    `yaml:"inactive_debounce_seconds,omitempty"`
}

const DefaultActiveDelaySeconds = 30

const DefaultInactiveDebounceSeconds = 60

// discordWebhookURLPattern catches a mistyped URL at load; Discord
// itself checks the token when a post is sent.
var discordWebhookURLPattern = regexp.MustCompile(`^https://discord\.com/api/webhooks/\d+/[A-Za-z0-9_-]+$`)

// validDigestCategories must match internal/api.validCategories; it's
// copied so config needn't import api.
var validDigestCategories = map[string]bool{
	"frags": true, "deaths": true, "kd_ratio": true, "matches": true,
	"captures": true, "flag_returns": true, "assists": true,
	"impressives": true, "excellents": true, "humiliations": true,
	"defends": true, "victories": true,
	"skulls_delivered": true, "obelisks_destroyed": true,
}

func validateDiscord(d *DiscordConfig) error {
	if d == nil {
		return nil
	}
	if d.LegacyWebhookURL != "" || len(d.LegacyDigestCategories) > 0 {
		return fmt.Errorf("discord.webhook_url / discord.digest_categories were moved to discord.digest.webhook_url / discord.digest.categories; update your config.yml")
	}
	if d.Digest != nil {
		if d.Digest.WebhookURL != "" && !discordWebhookURLPattern.MatchString(d.Digest.WebhookURL) {
			return fmt.Errorf("discord.digest.webhook_url %q does not match Discord webhook shape (https://discord.com/api/webhooks/{id}/{token})", d.Digest.WebhookURL)
		}
		for i, cat := range d.Digest.Categories {
			if !validDigestCategories[cat] {
				return fmt.Errorf("discord.digest.categories[%d] %q is not a valid leaderboard category", i, cat)
			}
		}
	}
	if d.Activity != nil {
		if d.Activity.WebhookURL != "" && !discordWebhookURLPattern.MatchString(d.Activity.WebhookURL) {
			return fmt.Errorf("discord.activity.webhook_url %q does not match Discord webhook shape (https://discord.com/api/webhooks/{id}/{token})", d.Activity.WebhookURL)
		}
		if d.Activity.ActiveDelaySeconds < 0 {
			return fmt.Errorf("discord.activity.active_delay_seconds must be >= 0 (got %d)", d.Activity.ActiveDelaySeconds)
		}
		if d.Activity.InactiveDebounceSeconds < 0 {
			return fmt.Errorf("discord.activity.inactive_debounce_seconds must be >= 0 (got %d)", d.Activity.InactiveDebounceSeconds)
		}
	}
	return nil
}

// ServerConfig holds HTTP server settings
type ServerConfig struct {
	ListenAddr   string        `yaml:"listen_addr"`
	HTTPPort     int           `yaml:"http_port"`
	PollInterval time.Duration `yaml:"poll_interval"`
	StaticDir    string        `yaml:"static_dir"`
	Quake3Dir    string        `yaml:"quake3_dir"`
	ServiceUser  string        `yaml:"service_user,omitempty"`
	UseSystemd   *bool         `yaml:"use_systemd,omitempty"`
}

// DatabaseConfig holds SQLite settings
type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// Q3Server is a game server to monitor. Key is its stable identifier,
// shown in the UI as "<source> / <key>": lowercase letters, digits, _
// and -.
//
// AllowHubAdminRcon, off by default, lets the hub's admins use RCON on
// this server. The source's owners can always use RCON.
type Q3Server struct {
	Key               string `yaml:"key"`
	Address           string `yaml:"address"`
	LogPath           string `yaml:"log_path"`
	RconPassword      string `yaml:"rcon_password"`
	AllowHubAdminRcon bool   `yaml:"allow_hub_admin_rcon"`
}

// Load reads configuration from a YAML file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Set defaults
	if cfg.Server.ListenAddr == "" {
		cfg.Server.ListenAddr = "127.0.0.1"
	}
	if cfg.Server.HTTPPort == 0 {
		cfg.Server.HTTPPort = 8080
	}
	if cfg.Server.PollInterval == 0 {
		cfg.Server.PollInterval = 5 * time.Second
	}
	// An empty StaticDir serves no static files.
	if cfg.Server.Quake3Dir == "" {
		cfg.Server.Quake3Dir = "/usr/lib/quake3"
	}

	applyTrackerDefaults(&cfg)

	// Database and Auth stay nil for a collector-only config.
	hasHub := cfg.Tracker == nil || cfg.Tracker.Hub != nil
	if hasHub {
		if cfg.Database == nil {
			cfg.Database = &DatabaseConfig{}
		}
		if cfg.Database.Path == "" {
			cfg.Database.Path = "/var/lib/trinity/trinity.db"
		}
		if cfg.Auth == nil {
			cfg.Auth = &AuthConfig{}
		}
		// An unset TokenDuration means JWTs never expire; users.token_version
		// ends web sessions, and a duration is an optional extra limit.
	}
	if err := validateTracker(cfg.Tracker); err != nil {
		return nil, err
	}

	for i, srv := range cfg.Q3Servers {
		if srv.Key == "" {
			return nil, fmt.Errorf("q3_servers[%d]: key is required", i)
		}
		if len(srv.Key) > 64 || !idPattern.MatchString(srv.Key) {
			return nil, fmt.Errorf("q3_servers[%d].key %q must match %s and be at most 64 chars", i, srv.Key, idPattern.String())
		}
	}

	if err := validateNoPlaceholders(&cfg); err != nil {
		return nil, err
	}

	if err := validateDiscord(cfg.Discord); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validateNoPlaceholders refuses a config that still has a literal
// "REPLACE-ME", as a hand-written or copied example config might. The
// wizard never writes one.
func validateNoPlaceholders(cfg *Config) error {
	const placeholder = "REPLACE-ME"
	if cfg.Tracker != nil && cfg.Tracker.Collector != nil {
		c := cfg.Tracker.Collector
		if strings.Contains(c.SourceID, placeholder) {
			return fmt.Errorf("tracker.collector.source_id is still %q; edit your config.yml", c.SourceID)
		}
		if strings.Contains(c.PublicURL, placeholder) {
			return fmt.Errorf("tracker.collector.public_url is still %q; edit your config.yml", c.PublicURL)
		}
	}
	for i, srv := range cfg.Q3Servers {
		if strings.Contains(srv.Address, placeholder) {
			return fmt.Errorf("q3_servers[%d].address is still %q; edit your config.yml", i, srv.Address)
		}
		if strings.Contains(srv.RconPassword, placeholder) {
			return fmt.Errorf("q3_servers[%d].rcon_password is still %q; edit your config.yml", i, srv.RconPassword)
		}
	}
	if cfg.Discord != nil {
		if cfg.Discord.Digest != nil && strings.Contains(cfg.Discord.Digest.WebhookURL, placeholder) {
			return fmt.Errorf("discord.digest.webhook_url is still %q; edit your config.yml", cfg.Discord.Digest.WebhookURL)
		}
		if cfg.Discord.Activity != nil && strings.Contains(cfg.Discord.Activity.WebhookURL, placeholder) {
			return fmt.Errorf("discord.activity.webhook_url is still %q; edit your config.yml", cfg.Discord.Activity.WebhookURL)
		}
	}
	return nil
}

// applyTrackerDefaults sets cfg.Tracker when absent and fills in each
// role's defaults, so cfg.Tracker is never nil afterward.
func applyTrackerDefaults(cfg *Config) {
	if cfg.Tracker == nil {
		// With no tracker block, run a hub and local collector on the
		// loopback. An explicit collector block must set public_url and
		// hub_host itself.
		cfg.Tracker = &TrackerConfig{
			Hub: &HubConfig{},
			Collector: &CollectorConfig{
				PublicURL: "http://127.0.0.1",
				HubHost:   "127.0.0.1",
			},
		}
	}
	t := cfg.Tracker
	if t.Hub != nil {
		if t.Hub.DedupWindow == 0 {
			t.Hub.DedupWindow = Duration(30 * time.Minute)
		}
		if t.Hub.Retention == 0 {
			t.Hub.Retention = Duration(10 * 24 * time.Hour)
		}
		if t.Hub.Directory != nil {
			d := t.Hub.Directory
			if d.Port == 0 {
				d.Port = 27950
			}
			if d.HeartbeatExpiry == 0 {
				d.HeartbeatExpiry = Duration(15 * time.Minute)
			}
			if d.ChallengeTimeout == 0 {
				d.ChallengeTimeout = Duration(2 * time.Second)
			}
			if d.GateRefresh == 0 {
				d.GateRefresh = Duration(10 * time.Second)
			}
			if d.MaxServers == 0 {
				d.MaxServers = 4096
			}
			if d.PersistedFreshness == 0 {
				d.PersistedFreshness = Duration(5 * time.Minute)
			}
		}
	}
	if t.Collector != nil {
		if t.Collector.HeartbeatInterval == 0 {
			t.Collector.HeartbeatInterval = Duration(30 * time.Second)
		}
		if t.Collector.LiveDelay == 0 {
			t.Collector.LiveDelay = Duration(5 * time.Second)
		}
		if t.Collector.DataDir == "" {
			// Next to the SQLite database, a directory main.go creates
			// on a hub.
			if cfg.Database != nil && cfg.Database.Path != "" {
				t.Collector.DataDir = dirOf(cfg.Database.Path)
			} else {
				t.Collector.DataDir = "/var/lib/trinity"
			}
		}
		if t.Collector.SourceID == "" && t.Hub != nil {
			// The hub upserts its local source at startup. A remote
			// collector must set source_id to its approved name.
			t.Collector.SourceID = "local"
		}
	}
	// A hub uses localhost and a collector-only config uses HubHost,
	// unless the YAML sets nats.url.
	if t.NATS.URL == "" {
		if t.Hub != nil {
			t.NATS.URL = "nats://localhost:4222"
		} else if t.Collector != nil && t.Collector.HubHost != "" {
			t.NATS.URL = "nats://" + t.Collector.HubHost + ":4222"
		} else {
			t.NATS.URL = "nats://localhost:4222"
		}
	}
}

func dirOf(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		if i == 0 {
			return "/"
		}
		return path[:i]
	}
	return "."
}

func validateTracker(t *TrackerConfig) error {
	if t == nil {
		return fmt.Errorf("tracker: internal error: applyTrackerDefaults must run first")
	}
	if t.Hub == nil && t.Collector == nil {
		return fmt.Errorf("tracker: must set at least one of hub or collector")
	}
	if t.Collector != nil {
		if t.Collector.SourceID == "" {
			return fmt.Errorf("tracker.collector.source_id is required (admin chose this name at provisioning)")
		}
		if t.Collector.DataDir == "" {
			return fmt.Errorf("tracker.collector.data_dir is required")
		}
		if t.Collector.PublicURL == "" {
			return fmt.Errorf("tracker.collector.public_url is required (publicly-reachable URL for this collector's host)")
		}
		u, err := url.Parse(t.Collector.PublicURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			return fmt.Errorf("tracker.collector.public_url must be an http(s) URL with a hostname (got %q)", t.Collector.PublicURL)
		}
		if t.Collector.HubHost == "" {
			return fmt.Errorf("tracker.collector.hub_host is required (the trinity hub this collector reports to)")
		}
	}
	return nil
}

// ValidateForSave runs Load's defaults and validators on an in-memory
// Config, so the wizard can check what it built before
// /etc/trinity/config.yml exists.
func ValidateForSave(cfg *Config) error {
	applyTrackerDefaults(cfg)
	if err := validateTracker(cfg.Tracker); err != nil {
		return err
	}
	for i, srv := range cfg.Q3Servers {
		if srv.Key == "" {
			return fmt.Errorf("q3_servers[%d]: key is required", i)
		}
		if len(srv.Key) > 64 || !idPattern.MatchString(srv.Key) {
			return fmt.Errorf("q3_servers[%d].key %q must match %s and be at most 64 chars", i, srv.Key, idPattern.String())
		}
	}
	return validateNoPlaceholders(cfg)
}

// Save writes the configuration to a YAML file, backing up the original first.
func Save(path string, cfg *Config) error {
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading existing config for backup: %w", err)
		}
		if err := os.WriteFile(path+".bak", data, 0640); err != nil {
			return fmt.Errorf("writing backup: %w", err)
		}
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0640); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

// AddServer appends a server to the config's Q3Servers slice
func AddServer(cfg *Config, server Q3Server) {
	cfg.Q3Servers = append(cfg.Q3Servers, server)
}

// RemoveServerByKey removes a server by key (case-insensitive) and
// returns whether it was found.
func RemoveServerByKey(cfg *Config, key string) bool {
	for i, s := range cfg.Q3Servers {
		if strings.EqualFold(s.Key, key) {
			cfg.Q3Servers = append(cfg.Q3Servers[:i], cfg.Q3Servers[i+1:]...)
			return true
		}
	}
	return false
}
