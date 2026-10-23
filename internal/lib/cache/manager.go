package cache

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/dont-be-evil-company/withsecrets/internal/lib/log"
)

// Manager handles caching operations with global and environment-specific settings
type Manager struct {
	cache        *Cache
	globalConfig *GlobalConfig
}

// GlobalConfig represents the global ws configuration
type GlobalConfig struct {
	Cache CacheConfig `yaml:"cache"`
}

// NewManager creates a new cache manager
func NewManager(globalConfig *GlobalConfig) (*Manager, error) {
	logger := log.NewLogger()

	// Only initialize cache if enabled globally
	if !globalConfig.Cache.Enabled {
		logger.Debug("Caching is disabled globally")
		return &Manager{
			cache:        nil,
			globalConfig: globalConfig,
		}, nil
	}

	// Initialize cache
	cache, err := NewCache()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cache: %w", err)
	}

	logger.Debug("Cache manager initialized", "enabled", true, "ttl", globalConfig.Cache.TTL)
	return &Manager{
		cache:        cache,
		globalConfig: globalConfig,
	}, nil
}

// Close closes the cache manager
func (m *Manager) Close() error {
	if m.cache != nil {
		return m.cache.Close()
	}
	return nil
}

// IsEnabled returns true if caching is enabled
func (m *Manager) IsEnabled() bool {
	return m.cache != nil && m.globalConfig.Cache.Enabled
}

// GetCacheConfig returns the effective cache configuration for an environment
func (m *Manager) GetCacheConfig(envCache *CacheConfig) (bool, time.Duration) {
	// Check if caching is disabled globally
	if !m.globalConfig.Cache.Enabled {
		return false, 0
	}

	// Use global TTL as default
	enabled := m.globalConfig.Cache.Enabled
	ttl := m.globalConfig.Cache.TTL

	// Override with environment-specific settings if present
	if envCache != nil {
		enabled = envCache.Enabled
		ttl = envCache.TTL
	}

	return enabled, ttl
}

// Get retrieves a secret from cache
func (m *Manager) Get(configPath, envName, secretName string) (string, bool, error) {
	if !m.IsEnabled() {
		return "", false, nil
	}

	// Get absolute path for consistent caching
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return "", false, fmt.Errorf("failed to get absolute path: %w", err)
	}

	return m.cache.Get(absPath, envName, secretName)
}

// Set stores a secret in cache
func (m *Manager) Set(configPath, envName, secretName, value string, ttl time.Duration) error {
	if !m.IsEnabled() {
		return nil
	}

	// Get absolute path for consistent caching
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	return m.cache.Set(absPath, envName, secretName, value, ttl)
}

// Delete removes a single cached secret
func (m *Manager) Delete(configPath, envName, secretName string) error {
	if !m.IsEnabled() {
		return nil
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	return m.cache.Delete(absPath, envName, secretName)
}

// Clear clears all cached secrets
func (m *Manager) Clear() error {
	if !m.IsEnabled() {
		return nil
	}
	return m.cache.Clear()
}

// ClearByPath clears all cached secrets for a specific ws.yaml file
func (m *Manager) ClearByPath(configPath string) error {
	if !m.IsEnabled() {
		return nil
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	return m.cache.ClearByPath(absPath)
}

// ClearByEnvironment clears all cached secrets for a specific environment
func (m *Manager) ClearByEnvironment(configPath, envName string) error {
	if !m.IsEnabled() {
		return nil
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	return m.cache.ClearByEnvironment(absPath, envName)
}

// List returns all cached entries (for debugging)
func (m *Manager) List() ([]CacheEntry, error) {
	if !m.IsEnabled() {
		return []CacheEntry{}, nil
	}
	return m.cache.List()
}

// GetStats returns cache statistics
func (m *Manager) GetStats() (map[string]interface{}, error) {
	if !m.IsEnabled() {
		return map[string]interface{}{
			"enabled": false,
		}, nil
	}

	entries, err := m.List()
	if err != nil {
		return nil, err
	}

	expanded := ExpandListEntries(entries)

	// Count entries by environment
	envCounts := make(map[string]int)
	for _, entry := range expanded {
		envCounts[entry.ConfigEnv]++
	}

	return map[string]interface{}{
		"enabled":            true,
		"total_entries":      len(expanded),
		"environment_counts": envCounts,
		"ttl":                m.globalConfig.Cache.TTL.String(),
	}, nil
}
