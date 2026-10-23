package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dont-be-evil-company/withsecrets/internal/lib/cache"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/log"
	"gopkg.in/yaml.v3"
)

// GlobalConfig represents the global ws configuration
type GlobalConfig struct {
	Cache    cache.CacheConfig `yaml:"cache"`
	Defaults *DefaultsConfig   `yaml:"defaults,omitempty"`
}

type DefaultsConfig struct {
	Providers map[string]ProviderDefaults `yaml:"providers,omitempty"`
}

type ProviderDefaults struct {
	Regions []string `yaml:"regions,omitempty"`
}

// UnmarshalYAML implements custom YAML unmarshaling for GlobalConfig
func (g *GlobalConfig) UnmarshalYAML(value *yaml.Node) error {
	// First, try to decode as a normal struct
	type rawGlobalConfig struct {
		Cache    interface{}     `yaml:"cache"`
		Defaults *DefaultsConfig `yaml:"defaults"`
	}

	var raw rawGlobalConfig
	if err := value.Decode(&raw); err != nil {
		return err
	}

	// Parse cache configuration
	if raw.Cache != nil {
		// Handle different cache formats
		switch cacheValue := raw.Cache.(type) {
		case map[string]interface{}:
			// Handle {enabled: true, ttl: "1d"} format
			if enabled, ok := cacheValue["enabled"].(bool); ok {
				g.Cache.Enabled = enabled
			}
			if ttlValue, ok := cacheValue["ttl"]; ok {
				duration, _, err := cache.ParseDuration(ttlValue)
				if err != nil {
					return fmt.Errorf("failed to parse cache TTL: %w", err)
				}
				g.Cache.TTL = duration
			}
		default:
			// Handle scalar values like "true", "1d", etc.
			duration, enabled, err := cache.ParseDuration(cacheValue)
			if err != nil {
				return fmt.Errorf("failed to parse cache configuration: %w", err)
			}
			g.Cache.Enabled = enabled
			g.Cache.TTL = duration
		}
	}

	g.Defaults = raw.Defaults

	return nil
}

// DefaultGlobalConfig returns the default global configuration
func DefaultGlobalConfig() *GlobalConfig {
	return &GlobalConfig{
		Cache: cache.CacheConfig{
			Enabled: false,          // Default to disabled for security
			TTL:     12 * time.Hour, // Default 12 hours when enabled
		},
	}
}

// LoadGlobalConfig loads the global configuration from .config/withsecrets/config.yaml
func LoadGlobalConfig() (*GlobalConfig, error) {
	logger := log.NewLogger()

	// Get user home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	configPath := filepath.Join(GlobalConfigDir(homeDir), "config.yaml")
	logger.Debug("Loading global configuration", "path", configPath)

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		logger.Debug("Global configuration file not found, using defaults")
		return DefaultGlobalConfig(), nil
	}

	// Read file
	data, err := os.ReadFile(configPath)
	if err != nil {
		logger.Debug("Failed to read global configuration file", "path", configPath, "error", err)
		return nil, fmt.Errorf("failed to read global configuration file: %w", err)
	}

	// Parse YAML
	var config GlobalConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		logger.Debug("Failed to parse global configuration YAML", "error", err)
		return nil, fmt.Errorf("failed to parse global configuration: %w", err)
	}

	// Validate and set defaults
	if config.Cache.TTL == 0 {
		config.Cache.TTL = 12 * time.Hour
	}

	logger.Debug("Global configuration loaded successfully", "cache_enabled", config.Cache.Enabled, "cache_ttl", config.Cache.TTL)
	return &config, nil
}

// SaveGlobalConfig saves the global configuration to .config/withsecrets/config.yaml
func SaveGlobalConfig(config *GlobalConfig) error {
	logger := log.NewLogger()

	// Get user home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDir := GlobalConfigDir(homeDir)
	configPath := filepath.Join(configDir, "config.yaml")

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal global configuration: %w", err)
	}

	// Write file
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		logger.Debug("Failed to write global configuration file", "path", configPath, "error", err)
		return fmt.Errorf("failed to write global configuration file: %w", err)
	}

	logger.Debug("Global configuration saved successfully", "path", configPath)
	return nil
}

// GetCacheDir returns the cache directory path
func GetCacheDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	cacheDir := CacheDir(homeDir)

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	return cacheDir, nil
}
