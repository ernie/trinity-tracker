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

	// Auth defaults
	if cfg.Auth.TokenDuration == 0 {
		cfg.Auth.TokenDuration = 24 * time.Hour
	}

	return &cfg, nil
}
