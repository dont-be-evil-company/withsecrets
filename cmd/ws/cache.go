package ws

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/cache"
	"github.com/spf13/cobra"
)

var (
	cachePath    string
	cacheName    string
	cacheEnv     string
	cacheVerbose bool
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage ws cache",
	Long: `Manage the withsecrets secrets cache.

This command allows you to:
- List cached secrets
- Clear cache entries
- Show cache statistics
- Configure cache settings

The cache is stored in ~/.cache/withsecrets/db.sqlite and helps reduce API calls
to cloud providers by storing secrets temporarily.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCacheCommand()
	},
}

var cacheListCmd = &cobra.Command{
	Use:   "list",
	Short: "List cached secrets",
	Long:  "List all cached secrets with their metadata.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCacheList()
	},
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear cached secrets",
	Long: `Clear cached secrets.

By default, clears secrets from ./ws.yaml in the current directory. Use filters to clear specific entries:
- --path: Clear secrets for a specific ws.yaml file (defaults to ./ws.yaml)
- --env: Clear secrets for a specific withsecrets environment
- --name: Clear secrets for a specific environment name
- --all: Clear all cached secrets from all paths
- --expired: Clear only expired secrets`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCacheClear(cmd)
	},
}

var cacheStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show cache statistics",
	Long:  "Show cache statistics including entry counts and configuration.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCacheStats()
	},
}

var cacheExpireCmd = &cobra.Command{
	Use:   "expire",
	Short: "Set expiry time for cached secrets",
	Long: `Set expiry time for cached secrets.

This command allows you to update the expiry time for existing cache entries.
You can filter by path, env, or name, and set a new expiry time using
human-readable format (e.g., "2w", "1d", "72h", "1y").

Examples:
  ws cache expire --path ws.yaml --ttl 2w
  ws cache expire --name production --ttl 1d
  ws cache expire --env staging --ttl 72h`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCacheExpire(cmd)
	},
}

var configCacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Configure cache settings",
	Long: `Configure global cache settings.

Examples:
  ws config cache --enable --ttl 1d
  ws config cache --disable
  ws config cache --ttl 2w`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCacheConfigWithCmd(cmd)
	},
}

func init() {
	// Add cache command to root
	rootCmd.AddCommand(cacheCmd)

	// Add subcommands to cache
	cacheCmd.AddCommand(cacheListCmd)
	cacheCmd.AddCommand(cacheClearCmd)
	cacheCmd.AddCommand(cacheStatsCmd)
	cacheCmd.AddCommand(cacheExpireCmd)

	// Global flags for cache commands
	cacheCmd.PersistentFlags().StringVarP(&cachePath, "path", "p", "", "Path to ws.yaml file")
	cacheCmd.PersistentFlags().StringVarP(&cacheEnv, "env", "e", "", "withsecrets environment name")
	cacheCmd.PersistentFlags().StringVarP(&cacheName, "name", "n", "", "Environment name")
	cacheCmd.PersistentFlags().BoolVarP(&cacheVerbose, "verbose", "v", false, "Verbose output")

	// Cache clear flags
	cacheClearCmd.Flags().Bool("all", false, "Clear all cached secrets")
	cacheClearCmd.Flags().Bool("expired", false, "Clear only expired secrets")

	// Cache expire flags
	cacheExpireCmd.Flags().String("ttl", "", "Set new expiry time (e.g., 2w, 1d, 72h, 1y)")
	cacheExpireCmd.MarkFlagRequired("ttl")

	// Cache config flags (moved to config command)
	configCacheCmd.Flags().Bool("enable", false, "Enable caching")
	configCacheCmd.Flags().Bool("disable", false, "Disable caching")
	configCacheCmd.Flags().String("ttl", "", "Set cache TTL (e.g., 1d, 2w, 72h, 2y)")
	configCacheCmd.Flags().Bool("show", false, "Show current configuration")
}

func runCacheCommand() error {
	fmt.Println("withsecrets Cache Management")
	fmt.Println("Use 'ws cache --help' to see available commands.")
	return nil
}

func runCacheList() error {
	// Load global config
	globalConfig, err := config.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("failed to load global config: %w", err)
	}

	// Convert to cache types
	cacheGlobalConfig := &cache.GlobalConfig{
		Cache: cache.CacheConfig{
			Enabled: globalConfig.Cache.Enabled,
			TTL:     globalConfig.Cache.TTL,
		},
	}

	// Initialize cache manager
	manager, err := cache.NewManager(cacheGlobalConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize cache manager: %w", err)
	}
	defer manager.Close()

	if !manager.IsEnabled() {
		fmt.Println("Caching is disabled.")
		return nil
	}

	// Get cached entries
	entries, err := manager.List()
	if err != nil {
		return fmt.Errorf("failed to list cache entries: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No cached secrets found.")
		return nil
	}

	// Filter entries if path or env specified
	filteredEntries := entries
	if cachePath != "" {
		absPath, err := filepath.Abs(cachePath)
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}
		var filtered []cache.CacheEntry
		for _, entry := range entries {
			if entry.Path == absPath {
				filtered = append(filtered, entry)
			}
		}
		filteredEntries = filtered
	}

	if cacheEnv != "" {
		var filtered []cache.CacheEntry
		for _, entry := range filteredEntries {
			if entry.ConfigEnv == cacheEnv {
				filtered = append(filtered, entry)
			}
		}
		filteredEntries = filtered
	}

	listed := cache.ExpandListEntries(filteredEntries)
	if len(listed) == 0 {
		fmt.Println("No cached secrets found.")
		return nil
	}

	fmt.Printf("Found %d cached secret(s):\n\n", len(listed))

	for _, entry := range listed {
		fmt.Printf("Path: %s\n", entry.Path)
		fmt.Printf("Environment: %s\n", entry.ConfigEnv)
		fmt.Printf("Variable: %s\n", entry.Env)
		if cacheVerbose {
			fmt.Printf("Value: %s\n", entry.Value)
		} else {
			fmt.Printf("Value: %s\n", maskSecret(entry.Value))
		}
		fmt.Printf("Created: %s\n", entry.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Expires: %s\n", entry.ExpiresAt.Format("2006-01-02 15:04:05"))
		fmt.Println(strings.Repeat("-", 50))
	}

	return nil
}

func runCacheClear(cmd *cobra.Command) error {
	// Get flags
	all, _ := cmd.Flags().GetBool("all")
	expired, _ := cmd.Flags().GetBool("expired")

	// Initialize cache
	cacheInstance, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}
	defer cacheInstance.Close()

	// Set default path if not provided
	pathToUse := cachePath
	if pathToUse == "" {
		// Get current working directory
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current working directory: %w", err)
		}
		pathToUse = filepath.Join(cwd, "ws.yaml")
	}

	// Determine what to clear
	var count int
	if all {
		// Clear all
		if err := cacheInstance.Clear(); err != nil {
			return fmt.Errorf("failed to clear cache: %w", err)
		}
		fmt.Println("Cleared all cached secrets.")
	} else {
		// Clear filtered entries
		count, err = cacheInstance.ClearFiltered(pathToUse, cacheEnv, cacheName, expired)
		if err != nil {
			return fmt.Errorf("failed to clear cache entries: %w", err)
		}

		// Show results
		fmt.Printf("Cleared %d cache entries\n", count)

		// Show filters used
		filters := []string{}
		if pathToUse != "" {
			filters = append(filters, fmt.Sprintf("path=%s", pathToUse))
		}
		if cacheEnv != "" {
			filters = append(filters, fmt.Sprintf("env=%s", cacheEnv))
		}
		if cacheName != "" {
			filters = append(filters, fmt.Sprintf("name=%s", cacheName))
		}
		if expired {
			filters = append(filters, "expired=true")
		}
		if len(filters) > 0 {
			fmt.Printf("Filters applied: %s\n", strings.Join(filters, ", "))
		}
	}

	return nil
}

func runCacheStats() error {
	// Load global config
	globalConfig, err := config.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("failed to load global config: %w", err)
	}

	// Convert to cache types
	cacheGlobalConfig := &cache.GlobalConfig{
		Cache: cache.CacheConfig{
			Enabled: globalConfig.Cache.Enabled,
			TTL:     globalConfig.Cache.TTL,
		},
	}

	// Initialize cache manager
	manager, err := cache.NewManager(cacheGlobalConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize cache manager: %w", err)
	}
	defer manager.Close()

	// Get stats
	stats, err := manager.GetStats()
	if err != nil {
		return fmt.Errorf("failed to get cache stats: %w", err)
	}

	fmt.Println("Cache Statistics:")
	fmt.Printf("Enabled: %v\n", stats["enabled"])

	if enabled, ok := stats["enabled"].(bool); ok && enabled {
		fmt.Printf("Total Entries: %v\n", stats["total_entries"])
		fmt.Printf("TTL: %v\n", stats["ttl"])

		if envCounts, ok := stats["environment_counts"].(map[string]int); ok {
			fmt.Println("Entries by Environment:")
			for env, count := range envCounts {
				fmt.Printf("  %s: %d\n", env, count)
			}
		}
	}

	return nil
}

func runCacheConfigWithCmd(cmd *cobra.Command) error {
	// Load current config
	globalConfig, err := config.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("failed to load global config: %w", err)
	}

	// Check if we should show current config
	show, _ := cmd.Flags().GetBool("show")
	if show {
		fmt.Println("Current Cache Configuration:")
		fmt.Printf("Enabled: %v\n", globalConfig.Cache.Enabled)
		fmt.Printf("TTL: %s\n", globalConfig.Cache.TTL)
		return nil
	}

	// Check for conflicting flags
	enable, _ := cmd.Flags().GetBool("enable")
	disable, _ := cmd.Flags().GetBool("disable")

	if enable && disable {
		return fmt.Errorf("cannot both enable and disable caching")
	}

	// Apply changes
	modified := false

	if enable {
		globalConfig.Cache.Enabled = true
		modified = true
	}

	if disable {
		globalConfig.Cache.Enabled = false
		modified = true
	}

	ttlStr, _ := cmd.Flags().GetString("ttl")
	if ttlStr != "" {
		duration, enabled, err := cache.ParseDuration(ttlStr)
		if err != nil {
			return fmt.Errorf("invalid TTL format: %w", err)
		}
		globalConfig.Cache.TTL = duration
		globalConfig.Cache.Enabled = enabled
		modified = true
	}

	if !modified {
		fmt.Println("No changes specified. Use --help to see available options.")
		return nil
	}

	// Save config
	if err := config.SaveGlobalConfig(globalConfig); err != nil {
		return fmt.Errorf("failed to save global config: %w", err)
	}

	fmt.Println("Cache configuration updated successfully.")
	fmt.Printf("Enabled: %v\n", globalConfig.Cache.Enabled)
	fmt.Printf("TTL: %s\n", globalConfig.Cache.TTL)

	return nil
}

func runCacheExpire(cmd *cobra.Command) error {
	// Get TTL from flag
	ttlStr, _ := cmd.Flags().GetString("ttl")
	if ttlStr == "" {
		return fmt.Errorf("--ttl is required")
	}

	// Parse TTL
	duration, _, err := cache.ParseDuration(ttlStr)
	if err != nil {
		return fmt.Errorf("invalid TTL format: %w", err)
	}

	// Initialize cache
	cacheInstance, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}
	defer cacheInstance.Close()

	// Update expiry for filtered entries
	count, err := cacheInstance.UpdateExpiry(cachePath, cacheEnv, cacheName, duration)
	if err != nil {
		return fmt.Errorf("failed to update cache expiry: %w", err)
	}

	// Show results
	fmt.Printf("Updated expiry for %d cache entries\n", count)
	fmt.Printf("New TTL: %s\n", duration)

	// Show filters used
	filters := []string{}
	if cachePath != "" {
		filters = append(filters, fmt.Sprintf("path=%s", cachePath))
	}
	if cacheEnv != "" {
		filters = append(filters, fmt.Sprintf("env=%s", cacheEnv))
	}
	if cacheName != "" {
		filters = append(filters, fmt.Sprintf("name=%s", cacheName))
	}
	if len(filters) > 0 {
		fmt.Printf("Filters applied: %s\n", strings.Join(filters, ", "))
	} else {
		fmt.Println("Applied to all cache entries")
	}

	return nil
}

// maskSecret masks a secret value for display
func maskSecret(value string) string {
	if len(value) == 0 {
		return ""
	}
	if len(value) <= 4 {
		return strings.Repeat("*", len(value))
	}
	return value[:2] + strings.Repeat("*", len(value)-4) + value[len(value)-2:]
}
