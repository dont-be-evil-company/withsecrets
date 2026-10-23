package guiapi

import (
	"time"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/cache"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/log"
)

func cacheSetSecret(configPath, envName, envVar, value string) {
	withCacheManager(configPath, envName, func(mgr *cache.Manager, ttl time.Duration) {
		logger := log.NewLogger()
		if err := mgr.Set(configPath, envName, envVar, value, ttl); err != nil {
			logger.Debug("Failed to update cache after secret write", "env_var", envVar, "error", err)
		}
		if err := mgr.PatchPathMappingEntry(configPath, envName, envVar, value); err != nil {
			logger.Debug("Failed to patch path mapping cache after secret write", "env_var", envVar, "error", err)
		}
	})
}

func cacheDeleteSecret(configPath, envName, envVar string) {
	withCacheManager(configPath, envName, func(mgr *cache.Manager, _ time.Duration) {
		logger := log.NewLogger()
		if err := mgr.Delete(configPath, envName, envVar); err != nil {
			logger.Debug("Failed to delete cached secret", "env_var", envVar, "error", err)
		}
		if err := mgr.RemovePathMappingEntry(configPath, envName, envVar); err != nil {
			logger.Debug("Failed to remove path mapping cache entry", "env_var", envVar, "error", err)
		}
	})
}

func cacheClearEnv(configPath, envName string) {
	withCacheManager(configPath, envName, func(mgr *cache.Manager, _ time.Duration) {
		logger := log.NewLogger()
		if err := mgr.ClearByEnvironment(configPath, envName); err != nil {
			logger.Debug("Failed to clear environment cache", "env", envName, "error", err)
		}
	})
}

func withCacheManager(configPath, envName string, fn func(*cache.Manager, time.Duration)) {
	logger := log.NewLogger()

	globalConfig, err := config.LoadGlobalConfig()
	if err != nil {
		logger.Debug("Failed to load global config for cache sync", "error", err)
		return
	}
	if !globalConfig.Cache.Enabled {
		return
	}

	mgr, err := cache.NewManager(&cache.GlobalConfig{Cache: globalConfig.Cache})
	if err != nil {
		logger.Debug("Failed to initialize cache manager for sync", "error", err)
		return
	}
	defer func() { _ = mgr.Close() }()

	var envCache *cache.CacheConfig
	if cfg, err := config.LoadSecretsConfig(configPath); err == nil {
		if env, err := cfg.GetEnvironment(envName); err == nil && env.Cache != nil {
			envCache = env.Cache
		}
	}

	enabled, ttl := mgr.GetCacheConfig(envCache)
	if !enabled {
		return
	}

	fn(mgr, ttl)
}
