package secrets

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/cache"
)

func hasProviderBackedMapping(envItems []config.EnvItem) bool {
	for _, envItem := range envItems {
		if envItem.Value != nil {
			continue
		}
		if envItem.SecretKey != "" || len(envItem.SecretPath) > 0 || envItem.ParamKey != "" || len(envItem.ParamPath) > 0 {
			return true
		}
	}
	return false
}

func tryRestoreCachedEnvironment(logger *slog.Logger, cacheManager *cache.Manager, env *config.Environment, envItems []config.EnvItem, configPath, envName string) (map[string]string, map[string]string, bool) {
	if !hasProviderBackedMapping(envItems) {
		return nil, nil, false
	}

	allSecrets := make(map[string]string)
	sourceRefs := make(map[string]string)

	for _, envItem := range envItems {
		if envItem.Value != nil || envItem.SecretKey == "" {
			continue
		}
		value, found, err := cacheManager.Get(configPath, envName, envItem.EnvironmentVariable)
		if err != nil {
			logger.Debug("Failed to get secret from cache", "env_var", envItem.EnvironmentVariable, "error", err)
			return nil, nil, false
		}
		if !found {
			logger.Debug("Secret not found in cache", "env_var", envItem.EnvironmentVariable)
			return nil, nil, false
		}
		allSecrets[envItem.EnvironmentVariable] = value
		sourceRefs[envItem.EnvironmentVariable] = envItem.SecretKey
		logger.Debug("Retrieved secret from cache", "env_var", envItem.EnvironmentVariable)
	}

	for _, envItem := range envItems {
		if envItem.Value != nil || len(envItem.SecretPath) == 0 {
			continue
		}
		if !restorePathMapping(logger, cacheManager, env, envItem, configPath, envName, pathMappingKindSecret, envItem.SecretPath, allSecrets, sourceRefs) {
			return nil, nil, false
		}
	}

	for _, envItem := range envItems {
		if envItem.Value != nil || envItem.ParamKey == "" {
			continue
		}
		value, found, err := cacheManager.Get(configPath, envName, envItem.EnvironmentVariable)
		if err != nil {
			logger.Debug("Failed to get parameter from cache", "env_var", envItem.EnvironmentVariable, "error", err)
			return nil, nil, false
		}
		if !found {
			logger.Debug("Parameter not found in cache", "env_var", envItem.EnvironmentVariable)
			return nil, nil, false
		}
		allSecrets[envItem.EnvironmentVariable] = value
		sourceRefs[envItem.EnvironmentVariable] = envItem.ParamKey
		logger.Debug("Retrieved parameter from cache", "env_var", envItem.EnvironmentVariable)
	}

	for _, envItem := range envItems {
		if envItem.Value != nil || len(envItem.ParamPath) == 0 {
			continue
		}
		if !restorePathMapping(logger, cacheManager, env, envItem, configPath, envName, pathMappingKindParam, envItem.ParamPath, allSecrets, sourceRefs) {
			return nil, nil, false
		}
	}

	return allSecrets, sourceRefs, true
}

func restorePathMapping(logger *slog.Logger, cacheManager *cache.Manager, env *config.Environment, envItem config.EnvItem, configPath, envName string, kind pathMappingKind, paths []string, allSecrets, sourceRefs map[string]string) bool {
	key := resolveFetchKey(env, envItem)
	id := pathMappingCacheID(kind, key, envItem.EnvironmentVariable, paths)
	mapping, found, err := cacheManager.GetPathMapping(configPath, envName, id)
	if err != nil {
		logger.Debug("Failed to get path mapping from cache", "mapping_type", string(kind), "mapping_key", envItem.EnvironmentVariable, "error", err)
		return false
	}
	if !found {
		logger.Debug("Path mapping cache miss", "mapping_type", string(kind), "provider", key.provider, "mapping_key", envItem.EnvironmentVariable, "paths", paths)
		return false
	}
	logger.Debug("Path mapping cache hit", "mapping_type", string(kind), "provider", key.provider, "mapping_key", envItem.EnvironmentVariable, "paths", paths, "entries", len(mapping.Entries))
	applyCachedPathEntries(allSecrets, sourceRefs, mapping)
	logger.Debug("Restored path mapping from cache", "mapping_type", string(kind), "provider", key.provider, "mapping_key", envItem.EnvironmentVariable, "entries", len(mapping.Entries))
	return true
}

func cacheResolvedEnvironment(logger *slog.Logger, cacheManager *cache.Manager, env *config.Environment, envItems []config.EnvItem, configPath, envName string, cacheTTL time.Duration, allSecrets map[string]string, secretPathCaptures, paramPathCaptures map[string]*capturedPathMapping) {
	cachedCount := 0
	for _, envItem := range envItems {
		if envItem.Value != nil {
			continue
		}

		if envItem.SecretKey != "" {
			if value, exists := allSecrets[envItem.EnvironmentVariable]; exists {
				if err := cacheManager.Set(configPath, envName, envItem.EnvironmentVariable, value, cacheTTL); err != nil {
					logger.Debug("Failed to cache secret", "env_var", envItem.EnvironmentVariable, "error", err)
				} else {
					cachedCount++
				}
			}
		}

		if envItem.ParamKey != "" {
			if value, exists := allSecrets[envItem.EnvironmentVariable]; exists {
				if err := cacheManager.Set(configPath, envName, envItem.EnvironmentVariable, value, cacheTTL); err != nil {
					logger.Debug("Failed to cache parameter", "env_var", envItem.EnvironmentVariable, "error", err)
				} else {
					cachedCount++
				}
			}
		}

		if len(envItem.SecretPath) > 0 {
			if writePathMapping(logger, cacheManager, env, envItem, configPath, envName, cacheTTL, pathMappingKindSecret, envItem.SecretPath, allSecrets, secretPathCaptures) {
				cachedCount++
			}
		}

		if len(envItem.ParamPath) > 0 {
			if writePathMapping(logger, cacheManager, env, envItem, configPath, envName, cacheTTL, pathMappingKindParam, envItem.ParamPath, allSecrets, paramPathCaptures) {
				cachedCount++
			}
		}
	}
	logger.Debug("Cached secrets", "count", cachedCount, "ttl", cacheTTL)
}

func writePathMapping(logger *slog.Logger, cacheManager *cache.Manager, env *config.Environment, envItem config.EnvItem, configPath, envName string, cacheTTL time.Duration, kind pathMappingKind, paths []string, allSecrets map[string]string, captures map[string]*capturedPathMapping) bool {
	key := resolveFetchKey(env, envItem)
	id := pathMappingCacheID(kind, key, envItem.EnvironmentVariable, paths)
	captured := captures[id]
	if captured == nil || !captured.complete {
		logger.Debug("Skipping incomplete path mapping cache write", "mapping_type", string(kind), "provider", key.provider, "mapping_key", envItem.EnvironmentVariable, "paths", paths)
		return false
	}
	mapping := toCachedPathMapping(capturedEntries(*captured), allSecrets)
	if err := cacheManager.SetPathMapping(configPath, envName, id, mapping, cacheTTL); err != nil {
		logger.Debug("Failed to cache path mapping", "mapping_type", string(kind), "mapping_key", envItem.EnvironmentVariable, "error", err)
		return false
	}
	logger.Debug("Cached resolved path mapping", "mapping_type", string(kind), "provider", key.provider, "mapping_key", envItem.EnvironmentVariable, "paths", paths, "entries", len(mapping.Entries))
	return true
}

func applyStaticValues(allSecrets map[string]string, envItems []config.EnvItem) {
	for _, envItem := range envItems {
		if envItem.Value == nil {
			continue
		}
		var strValue string
		switch v := envItem.Value.(type) {
		case string:
			strValue = v
		case int, int32, int64:
			strValue = fmt.Sprintf("%d", v)
		case float32, float64:
			strValue = fmt.Sprintf("%g", v)
		default:
			strValue = fmt.Sprintf("%v", v)
		}
		allSecrets[envItem.EnvironmentVariable] = strValue
	}
}

func interpolateAll(allSecrets map[string]string) {
	for key, value := range allSecrets {
		if strings.Contains(value, "${") {
			allSecrets[key] = config.InterpolateEnvVars(value, allSecrets)
		}
	}
}
