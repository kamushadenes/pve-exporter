package config

import (
	"flag"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the application configuration
type Config struct {
	Proxmox ProxmoxConfig `yaml:"proxmox"`
	Server  ServerConfig  `yaml:"server"`
}

// ProxmoxConfig holds Proxmox API configuration
type ProxmoxConfig struct {
	Host               string        `yaml:"host"`
	Port               int           `yaml:"port"`
	User               string        `yaml:"user"`
	Password           string        `yaml:"password"`
	TokenID            string        `yaml:"token_id"`
	TokenSecret        string        `yaml:"token_secret"`
	Realm              string        `yaml:"realm"`
	InsecureSkipVerify bool          `yaml:"insecure_skip_verify"`
	Timeout            time.Duration `yaml:"timeout"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	ListenAddress string `yaml:"listen_address"`
	MetricsPath   string `yaml:"metrics_path"`
}

// Load loads configuration from file and environment variables
func Load() (*Config, error) {
	var configFile string
	flag.StringVar(&configFile, "config", "", "Path to configuration file")
	flag.Parse()

	// Default configuration
	cfg := &Config{
		Proxmox: ProxmoxConfig{
			Host:               getEnv("PVE_HOST", "localhost"),
			Port:               8006,
			User:               getEnv("PVE_USER", "root@pam"),
			Password:           getEnv("PVE_PASSWORD", ""),
			TokenID:            getEnv("PVE_TOKEN_ID", ""),
			TokenSecret:        getEnv("PVE_TOKEN_SECRET", ""),
			Realm:              getEnv("PVE_REALM", "pam"),
			InsecureSkipVerify: getEnvBool("PVE_INSECURE_SKIP_VERIFY", true),
			Timeout:            30 * time.Second,
		},
		Server: ServerConfig{
			ListenAddress: getEnv("LISTEN_ADDRESS", ":9221"),
			MetricsPath:   getEnv("METRICS_PATH", "/metrics"),
		},
	}

	// Load from file if specified
	if configFile != "" {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Proxmox.Host == "" {
		return fmt.Errorf("proxmox host is required")
	}

	// Check authentication method
	hasPassword := c.Proxmox.Password != ""
	hasToken := c.Proxmox.TokenID != "" && c.Proxmox.TokenSecret != ""

	if !hasPassword && !hasToken {
		return fmt.Errorf("either password or token authentication must be configured")
	}

	return nil
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvBool gets a boolean environment variable or returns a default value
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}
