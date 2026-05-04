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

// idPattern: lowercase alnum + underscore + hyphen. Used to validate
// q3_servers[].key (and source IDs in tests). Lowercase-only so the
// hub identity matches the filesystem paths (which are always
// lowercased). Kept here to avoid pulling storage into config's
// import graph.
var idPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

// Config holds the application configuration.
//
// Tracker is always populated after Load: if the YAML omits the block
// entirely, Load fills it with a hub+local-collector default. A
// private single-server install just listens on 127.0.0.1; the
// distinction between "standalone" and "distributed" is no longer
// modeled at the config layer.
type Config struct {
	Server ServerConfig `yaml:"server"`
	// Database / Auth are pointers so collector-only configs can omit
	// the blocks entirely. config.Load fills hub-mode defaults; read
	// sites must nil-check.
	Database  *DatabaseConfig `yaml:"database,omitempty"`
	Auth      *AuthConfig     `yaml:"auth,omitempty"`
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
	// Accept a trailing "d" by converting it to hours before handing off to stdlib.
	if strings.HasSuffix(s, "d") {
		n, err := strconv.ParseInt(strings.TrimSuffix(s, "d"), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// TrackerConfig selects the role(s) this process takes. Presence of
// Hub / Collector picks between:
//
//   - Hub + local collector (both set) — single-machine default.
//   - Hub-only (Hub set, Collector nil) — community hub that only
//     accepts remote collectors.
//   - Collector-only (Collector set, Hub nil) — remote log parser
//     feeding a hub over NATS.
type TrackerConfig struct {
	NATS      NATSConfig       `yaml:"nats"`
	Hub       *HubConfig       `yaml:"hub,omitempty"`
	Collector *CollectorConfig `yaml:"collector,omitempty"`
}

// NATSConfig points at the NATS endpoint. For a hub it is the bind address
// of the embedded server; for a remote collector it is the URL of the hub
// to connect to.
//
// CertFile and KeyFile, if both set, enable TLS on the embedded
// server. They are ignored in collector-only mode (clients learn TLS
// is required from the server's INFO message and upgrade
// transparently, validating the server cert against system roots).
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

// DirectoryConfig configures the optional Quake 3 directory (a.k.a.
// master) server. Off by default: even when the block is present,
// Enabled must be true before the hub binds UDP. Heartbeats are gated
// by membership in the servers table — only servers whose stored
// address resolves to the heartbeat's source IP:port are admitted.
type DirectoryConfig struct {
	Enabled          bool     `yaml:"enabled"`
	ListenAddr       string   `yaml:"listen_addr,omitempty"`
	Port             int      `yaml:"port,omitempty"`
	HeartbeatExpiry  Duration `yaml:"heartbeat_expiry,omitempty"`
	ChallengeTimeout Duration `yaml:"challenge_timeout,omitempty"`
	GateRefresh      Duration `yaml:"gate_refresh,omitempty"`
	MaxServers       int      `yaml:"max_servers,omitempty"`
	// PersistedFreshness gates restoration of the registry on
	// startup: if the persisted snapshot's newest validated_at is
	// older than this, it's treated as a crash artifact and
	// discarded. Tune up if your deploys take longer than the
	// default 5 minutes.
	PersistedFreshness Duration `yaml:"persisted_freshness,omitempty"`
}

// CollectorConfig configures the log-parser / publisher role.
//
// PublicURL is the publicly-reachable URL for this collector's host
// (e.g. https://nil.ernie.io). The hub stores it as the source's
// demo download base, used for cross-host asset fallback. Required.
//
// HubHost is the bare hostname of the trinity hub this collector
// reports to; in collector-only mode it supplies the default
// nats.url (nats://<hub_host>:4222). Required.
//
// Each q3_servers[].address should hold the publicly-reachable
// host:port for that server (i.e. matching PublicURL's hostname plus
// the q3 net_port). The collector uses that address for rcon and the
// hub uses it for UDP getstatus polling.
type CollectorConfig struct {
	SourceID          string   `yaml:"source_id"`
	DataDir           string   `yaml:"data_dir"`
	HeartbeatInterval Duration `yaml:"heartbeat_interval"`
	PublicURL         string   `yaml:"public_url"`
	HubHost           string   `yaml:"hub_host"`
}

// AuthConfig holds authentication settings
type AuthConfig struct {
	JWTSecret     string        `yaml:"jwt_secret"`
	TokenDuration time.Duration `yaml:"token_duration"`
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

// Q3Server represents a Quake 3 server to monitor. Key is the stable
// identifier shown in the UI as "<source> / <key>"; same character
// restrictions as a source name (alnum/underscore/hyphen).
//
// AllowHubAdminRcon opts this server in to admin delegation: when
// true, hub admins (is_admin=1 users on the hub this collector
// reports to) are allowed to RCON the server even though they don't
// own this collector's source. Owners of the source can RCON
// regardless of this flag. Default false — operators must explicitly
// hand the keys over.
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
	// Note: StaticDir intentionally has no default - empty means don't serve static files
	if cfg.Server.Quake3Dir == "" {
		cfg.Server.Quake3Dir = "/usr/lib/quake3"
	}

	applyTrackerDefaults(&cfg)

	// Hub-only defaults: Database/Auth stay nil for collector-only.
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
		if cfg.Auth.TokenDuration == 0 {
			cfg.Auth.TokenDuration = 24 * time.Hour
		}
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

	return &cfg, nil
}

// validateNoPlaceholders refuses configs where the operator forgot to
// edit literal "REPLACE-ME" placeholders. The wizard never writes
// these — but a hand-rolled config or one carried over from an
// example may still contain them, and without this guard the
// collector would silently publish against a bogus source name or
// rcon with the literal string "REPLACE-ME".
func validateNoPlaceholders(cfg *Config) error {
	const placeholder = "REPLACE-ME"
	if cfg.Tracker != nil && cfg.Tracker.Collector != nil {
		c := cfg.Tracker.Collector
		if strings.Contains(c.SourceID, placeholder) {
			return fmt.Errorf("tracker.collector.source_id is still %q — edit your config.yml", c.SourceID)
		}
		if strings.Contains(c.PublicURL, placeholder) {
			return fmt.Errorf("tracker.collector.public_url is still %q — edit your config.yml", c.PublicURL)
		}
	}
	for i, srv := range cfg.Q3Servers {
		if strings.Contains(srv.Address, placeholder) {
			return fmt.Errorf("q3_servers[%d].address is still %q — edit your config.yml", i, srv.Address)
		}
		if strings.Contains(srv.RconPassword, placeholder) {
			return fmt.Errorf("q3_servers[%d].rcon_password is still %q — edit your config.yml", i, srv.RconPassword)
		}
	}
	return nil
}

// applyTrackerDefaults populates cfg.Tracker if absent (single-machine
// hub+collector) and fills in per-role defaults. After this call
// cfg.Tracker is always non-nil.
func applyTrackerDefaults(cfg *Config) {
	if cfg.Tracker == nil {
		// Implicit single-host install with no tracker block: default to
		// hub+local-collector with the loopback advertised. Operators
		// who write an explicit collector block must set public_url and
		// hub_host themselves.
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
		if t.Collector.DataDir == "" {
			// Default alongside the SQLite DB; main.go already creates
			// this dir on hub deployments.
			if cfg.Database != nil && cfg.Database.Path != "" {
				t.Collector.DataDir = dirOf(cfg.Database.Path)
			} else {
				t.Collector.DataDir = "/var/lib/trinity"
			}
		}
		if t.Collector.SourceID == "" && t.Hub != nil {
			// Hub+local-collector: the local source is created/upserted
			// at startup, so source_id is internal metadata. Remote
			// collectors (no Hub set) must still supply it explicitly —
			// it's whatever name the hub approved (self-service request
			// or admin-direct create).
			t.Collector.SourceID = "local"
		}
	}
	// NATS URL: embedded hub or in-process collector use localhost;
	// collector-only mode derives from HubHost. Explicit nats.url in
	// the YAML always wins.
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
		return fmt.Errorf("tracker: internal — applyTrackerDefaults must run first")
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

// ValidateForSave runs every validator that Load applies (defaults +
// tracker validation + placeholder check) against an in-memory
// *Config. The wizard uses this to check the config it built before
// writing /etc/trinity/config.yml — round-tripping through Load would
// fail because the file doesn't exist yet.
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

// Save writes the configuration to a YAML file, backing up the original first
func Save(path string, cfg *Config) error {
	// Back up existing file
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
