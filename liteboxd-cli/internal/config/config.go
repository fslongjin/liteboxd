package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
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

// SaveToken writes the API token to the config file
func SaveToken(token string) error {
	if err := EnsureConfigDir(); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := GetConfigPath()
	data := make(map[string]interface{})

	// Read existing config if it exists
	content, err := os.ReadFile(configPath)
	if err == nil && len(content) > 0 {
		if err := yaml.Unmarshal(content, &data); err != nil {
			return fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	data["token"] = token

	out, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, out, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Update viper in-memory so subsequent calls pick it up
	viper.Set("token", token)
	return nil
}

// ClearToken removes the API token from the config file
func ClearToken() error {
	configPath := GetConfigPath()
	data := make(map[string]interface{})

	content, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to clear
		}
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if len(content) > 0 {
		if err := yaml.Unmarshal(content, &data); err != nil {
			return fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	delete(data, "token")

	out, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, out, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	viper.Set("token", "")
	return nil
}
