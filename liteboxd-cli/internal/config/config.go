package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds the application configuration
type Config struct {
	APIServer string
	Timeout   int
	Output    string
	Profiles  map[string]ProfileConfig
}

// ProfileConfig holds profile-specific configuration
type ProfileConfig struct {
	APIServer string `yaml:"api-server"`
	Token     string `yaml:"token,omitempty"`
}

// Load loads the configuration from file and environment
func Load() (*Config, error) {
	cfg := &Config{
		APIServer: viper.GetString("api-server"),
		Timeout:   int(viper.GetDuration("timeout").Seconds()),
		Output:    viper.GetString("output"),
	}

	return cfg, nil
}

// GetConfigPath returns the default config file path
func GetConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "liteboxd", "config.yaml")
}

// EnsureConfigDir ensures the config directory exists
func EnsureConfigDir() error {
	path := GetConfigPath()
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0755)
}
