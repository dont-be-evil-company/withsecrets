package cache

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dont-be-evil-company/withsecrets/internal/lib/log"
	_ "github.com/mattn/go-sqlite3"
)

const sqliteDateTime = "2006-01-02 15:04:05"

// Cache represents a SQLite-based cache for secrets
type Cache struct {
	db *sql.DB
}

// CacheEntry represents a cached secret entry
type CacheEntry struct {
	Path      string    `json:"path"`
	ConfigEnv string    `json:"config_env"`
	Env       string    `json:"env"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewCache creates a new cache instance
func NewCache() (*Cache, error) {
	logger := log.NewLogger()

	// Get cache directory
	cacheDir, err := getCacheDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get cache directory: %w", err)
	}

	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	dbPath := filepath.Join(cacheDir, "db.sqlite")
	logger.Debug("Opening cache database", "path", dbPath)

	// Open database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open cache database: %w", err)
	}

	cache := &Cache{db: db}

	// Initialize database schema
	if err := cache.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize cache schema: %w", err)
	}

	// Clean up expired entries
	if err := cache.cleanupExpired(); err != nil {
		logger.Debug("Failed to cleanup expired entries", "error", err)
		// Don't fail cache creation for cleanup errors
	}

	logger.Debug("Cache initialized successfully", "path", dbPath)
	return cache, nil
}

// Close closes the cache database connection
func (c *Cache) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// initSchema initializes the database schema
func (c *Cache) initSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS secrets (
		path TEXT NOT NULL,
		config_env TEXT NOT NULL,
		env TEXT NOT NULL,
		value TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME NOT NULL,
		PRIMARY KEY (path, config_env, env)
	);
	
	CREATE INDEX IF NOT EXISTS idx_expires_at ON secrets(expires_at);
	`

	_, err := c.db.Exec(query)
	return err
}

// cleanupExpired removes expired entries from the cache
func (c *Cache) cleanupExpired() error {
	query := `DELETE FROM secrets WHERE expires_at < datetime('now')`
	_, err := c.db.Exec(query)
	return err
}

// Set stores a secret in the cache
func (c *Cache) Set(path, configEnv, env, value string, ttl time.Duration) error {
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)

	query := `
	INSERT OR REPLACE INTO secrets (path, config_env, env, value, created_at, expires_at)
	VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := c.db.Exec(query, path, configEnv, env, value, now.Format(sqliteDateTime), expiresAt.Format(sqliteDateTime))
	return err
}

// Get retrieves a secret from the cache
func (c *Cache) Get(path, configEnv, env string) (string, bool, error) {
	query := `
	SELECT value FROM secrets 
	WHERE path = ? AND config_env = ? AND env = ? AND expires_at > datetime('now')
	`

	var value string
	err := c.db.QueryRow(query, path, configEnv, env).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}

	return value, true, nil
}

// Delete removes a secret from the cache
func (c *Cache) Delete(path, configEnv, env string) error {
	query := `DELETE FROM secrets WHERE path = ? AND config_env = ? AND env = ?`
	_, err := c.db.Exec(query, path, configEnv, env)
	return err
}

// Clear removes all secrets from the cache
func (c *Cache) Clear() error {
	query := `DELETE FROM secrets`
	_, err := c.db.Exec(query)
	return err
}

// ClearByPath removes all secrets for a specific ws.yaml path
func (c *Cache) ClearByPath(path string) error {
	query := `DELETE FROM secrets WHERE path = ?`
	_, err := c.db.Exec(query, path)
	return err
}

// ClearByEnvironment removes all secrets for a specific environment
func (c *Cache) ClearByEnvironment(path, configEnv string) error {
	query := `DELETE FROM secrets WHERE path = ? AND config_env = ?`
	_, err := c.db.Exec(query, path, configEnv)
	return err
}

// List returns all cached entries (for debugging/inspection)
func (c *Cache) List() ([]CacheEntry, error) {
	query := `
	SELECT path, config_env, env, value, created_at, expires_at
	FROM secrets
	ORDER BY path, config_env, env
	`

	rows, err := c.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []CacheEntry
	for rows.Next() {
		var entry CacheEntry
		err := rows.Scan(&entry.Path, &entry.ConfigEnv, &entry.Env, &entry.Value, &entry.CreatedAt, &entry.ExpiresAt)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// ClearFiltered clears cache entries based on filters
func (c *Cache) ClearFiltered(path, configEnv, env string, expiredOnly bool) (int, error) {
	logger := log.NewLogger()

	// Build WHERE clause based on filters
	var conditions []string
	var args []interface{}
	argIndex := 1

	if path != "" {
		conditions = append(conditions, fmt.Sprintf("path = $%d", argIndex))
		args = append(args, path)
		argIndex++
	}

	if configEnv != "" {
		conditions = append(conditions, fmt.Sprintf("config_env = $%d", argIndex))
		args = append(args, configEnv)
		argIndex++
	}

	if env != "" {
		conditions = append(conditions, fmt.Sprintf("env = $%d", argIndex))
		args = append(args, env)
		argIndex++
	}

	if expiredOnly {
		conditions = append(conditions, fmt.Sprintf("expires_at < $%d", argIndex))
		args = append(args, time.Now())
		argIndex++
	}

	// Build query
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf("DELETE FROM secrets %s", whereClause)

	result, err := c.db.Exec(query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to clear cache entries: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	logger.Debug("Cleared cache entries", "count", rowsAffected, "path", path, "config_env", configEnv, "env", env, "expired_only", expiredOnly)
	return int(rowsAffected), nil
}

// UpdateExpiry updates the expiry time for cache entries based on filters
func (c *Cache) UpdateExpiry(path, configEnv, env string, newTTL time.Duration) (int, error) {
	logger := log.NewLogger()

	// Build WHERE clause based on filters
	var conditions []string
	var args []interface{}
	argIndex := 1

	if path != "" {
		conditions = append(conditions, fmt.Sprintf("path = $%d", argIndex))
		args = append(args, path)
		argIndex++
	}

	if configEnv != "" {
		conditions = append(conditions, fmt.Sprintf("config_env = $%d", argIndex))
		args = append(args, configEnv)
		argIndex++
	}

	if env != "" {
		conditions = append(conditions, fmt.Sprintf("env = $%d", argIndex))
		args = append(args, env)
		argIndex++
	}

	// Build query - set new expiry time to now + TTL
	newExpiryTime := time.Now().Add(newTTL)
	conditions = append(conditions, fmt.Sprintf("expires_at = $%d", argIndex))
	args = append(args, newExpiryTime)
	argIndex++

	whereClause := ""
	if len(conditions) > 1 { // More than just the expiry condition
		whereClause = "WHERE " + strings.Join(conditions[:len(conditions)-1], " AND ")
	}

	query := fmt.Sprintf("UPDATE secrets SET expires_at = $%d %s", argIndex, whereClause)

	result, err := c.db.Exec(query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to update cache expiry: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	logger.Debug("Updated cache expiry", "count", rowsAffected, "path", path, "config_env", configEnv, "env", env, "new_ttl", newTTL, "new_expiry", newExpiryTime)
	return int(rowsAffected), nil
}

// getCacheDir returns the withsecrets cache directory path.
func getCacheDir() (string, error) {
	baseDir, err := cacheBaseDir()
	if err != nil {
		return "", fmt.Errorf("failed to get cache base directory: %w", err)
	}

	return filepath.Join(baseDir, "withsecrets"), nil
}

func cacheBaseDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Caches"), nil
	case "windows":
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return localAppData, nil
		}
		return filepath.Join(homeDir, "AppData", "Local"), nil
	default:
		if xdgCacheHome := os.Getenv("XDG_CACHE_HOME"); xdgCacheHome != "" {
			return xdgCacheHome, nil
		}
		return filepath.Join(homeDir, ".cache"), nil
	}
}
