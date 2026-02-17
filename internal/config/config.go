package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the application configuration
type Config struct {
	Server    ServerConfig   `yaml:"server"`
	Database  DatabaseConfig `yaml:"database"`
	Auth      AuthConfig     `yaml:"auth"`
	Q3Servers []Q3Server     `yaml:"q3_servers"`
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

	return &cfg, nil
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
