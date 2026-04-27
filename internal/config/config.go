package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the application configuration
type Config struct {
	Server    ServerConfig   `yaml:"server"`
	Database  DatabaseConfig `yaml:"database"`
	Auth      AuthConfig     `yaml:"auth"`
	Q3Servers []Q3Server     `yaml:"q3_servers"`
	Tracker   *TrackerConfig `yaml:"tracker,omitempty"`
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

// TrackerConfig enables distributed tracking. Nil means standalone mode (no
// NATS, no role split). Presence of the nested Hub / Collector pointers
// selects which role(s) the process takes on; both may be set for an
// in-process hub + local-collector deployment.
type TrackerConfig struct {
	NATS      NATSConfig       `yaml:"nats"`
	Hub       *HubConfig       `yaml:"hub,omitempty"`
	Collector *CollectorConfig `yaml:"collector,omitempty"`
}

// NATSConfig points at the NATS endpoint. For a hub it is the bind address
// of the embedded server; for a remote collector it is the URL of the hub
// to connect to.
type NATSConfig struct {
	URL             string `yaml:"url"`
	CredentialsFile string `yaml:"credentials_file,omitempty"`
}

// HubConfig configures the aggregator role.
type HubConfig struct {
	DedupWindow      Duration `yaml:"dedup_window"`
	Retention        Duration `yaml:"retention"`
	ApprovalRequired bool     `yaml:"approval_required"`
}

// CollectorConfig configures the log-parser / publisher role.
//
// HubHost is a bare hostname: it is shown verbatim in !claim chat
// replies and, when tracker.nats.url is unset, supplies the default
// NATS connect endpoint for collector-only mode
// (nats://<hub_host>:4222).
type CollectorConfig struct {
	SourceID          string   `yaml:"source_id"`
	DataDir           string   `yaml:"data_dir"`
	HeartbeatInterval Duration `yaml:"heartbeat_interval"`
	DemoBaseURL       string   `yaml:"demo_base_url,omitempty"`
	HubHost           string   `yaml:"hub_host,omitempty"`
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

// Q3Server represents a Quake 3 server to monitor
type Q3Server struct {
	Name         string `yaml:"name"`
	Address      string `yaml:"address"`
	LogPath      string `yaml:"log_path"`
	RconPassword string `yaml:"rcon_password"`
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
	if cfg.Database.Path == "" {
		cfg.Database.Path = "/var/lib/trinity/trinity.db"
	}
	// Note: StaticDir intentionally has no default - empty means don't serve static files
	if cfg.Server.Quake3Dir == "" {
		cfg.Server.Quake3Dir = "/usr/lib/quake3"
	}

	// Auth defaults
	if cfg.Auth.TokenDuration == 0 {
		cfg.Auth.TokenDuration = 24 * time.Hour
	}

	if err := applyTrackerDefaults(cfg.Tracker); err != nil {
		return nil, err
	}
	if err := validateTracker(cfg.Tracker); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func applyTrackerDefaults(t *TrackerConfig) error {
	if t == nil {
		return nil
	}
	if t.Hub != nil {
		if t.Hub.DedupWindow == 0 {
			t.Hub.DedupWindow = Duration(30 * time.Minute)
		}
		if t.Hub.Retention == 0 {
			t.Hub.Retention = Duration(10 * 24 * time.Hour)
		}
	}
	if t.Collector != nil {
		if t.Collector.HeartbeatInterval == 0 {
			t.Collector.HeartbeatInterval = Duration(30 * time.Second)
		}
		if t.Collector.HubHost == "" {
			t.Collector.HubHost = "trinity.run"
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
	return nil
}

func validateTracker(t *TrackerConfig) error {
	if t == nil {
		return nil
	}
	if t.Hub == nil && t.Collector == nil {
		return fmt.Errorf("tracker: must set at least one of hub or collector")
	}
	if t.Collector != nil {
		if t.Collector.SourceID == "" {
			return fmt.Errorf("tracker.collector.source_id is required")
		}
		if t.Collector.DataDir == "" {
			return fmt.Errorf("tracker.collector.data_dir is required")
		}
	}
	return nil
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

// RemoveServerByName removes a server by name and returns whether it was found
func RemoveServerByName(cfg *Config, name string) bool {
	for i, s := range cfg.Q3Servers {
		if s.Name == name {
			cfg.Q3Servers = append(cfg.Q3Servers[:i], cfg.Q3Servers[i+1:]...)
			return true
		}
	}
	return false
}
