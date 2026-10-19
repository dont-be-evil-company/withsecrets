package secrets

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mistweaverco/withsecrets/internal/config"
	"github.com/mistweaverco/withsecrets/internal/lib/cache"
	"github.com/mistweaverco/withsecrets/internal/lib/log"
)

// SecretManager defines the interface for secret management operations
type SecretManager interface {
	GetSecret(projectID, secretID string) (string, error)
	GetSecrets(projectID string, secretIDs []string) (map[string]string, error)
	GetSecretsByPath(projectID, secretPath string) (map[string]string, error)
	Close() error
}

// ParameterStore is an optional interface for providers that support AWS-style
// Parameter Store (or equivalent) param-key / param-path mappings.
type ParameterStore interface {
	GetParameter(projectID, paramName string) (string, error)
	GetParameters(projectID string, paramNames []string) (map[string]string, error)
	GetParametersByPath(projectID, paramPath string) (map[string]string, error)
	PutParameter(projectID, paramName, paramValue string) error
	DeleteParameter(projectID, paramName string) error
}

// SecretMutator is an optional interface implemented by providers that support
// creating/updating/deleting secrets (used by interactive tooling like the TUI).
type SecretMutator interface {
	CreateSecret(secretName, secretValue, description string) error
	UpdateSecret(secretName, secretValue string) error
	DeleteSecret(secretName string, forceDelete bool) error
}

// SecretManagerFactory creates secret managers for different cloud providers
type SecretManagerFactory struct {
	createManager func(ctx context.Context, provider, projectID, region string) (SecretManager, error)
}

// NewSecretManagerFactory creates a new secret manager factory
func NewSecretManagerFactory() *SecretManagerFactory {
	return &SecretManagerFactory{}
}

// CreateSecretManager creates a secret manager for the specified provider.
// region is used by AWS; when empty, AWS_REGION / AWS_DEFAULT_REGION are used.
func (f *SecretManagerFactory) CreateSecretManager(ctx context.Context, provider string, projectID string, region string) (SecretManager, error) {
	if f.createManager != nil {
		return f.createManager(ctx, provider, projectID, region)
	}
	switch provider {
	case "gcp":
		// Check for GCP credentials
		credentialsFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
		return NewGCPSecretManager(ctx, credentialsFile, projectID)
	case "aws":
		resolvedRegion := region
		if resolvedRegion == "" {
			resolvedRegion = os.Getenv("AWS_REGION")
		}
		if resolvedRegion == "" {
			resolvedRegion = os.Getenv("AWS_DEFAULT_REGION")
		}
		profile := os.Getenv("AWS_PROFILE")
		return NewAWSSecretsManager(ctx, resolvedRegion, profile)
	case "azure":
		// Check for Azure Key Vault configuration
		vaultURL := os.Getenv("AZURE_KEY_VAULT_URL")
		if vaultURL == "" {
			return nil, fmt.Errorf("AZURE_KEY_VAULT_URL environment variable is required for Azure Key Vault")
		}

		// Optional: tenant ID, client ID, and client secret for service principal auth
		tenantID := os.Getenv("AZURE_TENANT_ID")
		clientID := os.Getenv("AZURE_CLIENT_ID")
		clientSecret := os.Getenv("AZURE_CLIENT_SECRET")

		return NewAzureKeyVaultManager(ctx, vaultURL, tenantID, clientID, clientSecret)
	case "openbao":
		// Check for OpenBao configuration
		address := os.Getenv("OPENBAO_ADDR")
		if address == "" {
			return nil, fmt.Errorf("OPENBAO_ADDR environment variable is required for OpenBao")
		}

		// Optional: token and namespace
		token := os.Getenv("OPENBAO_TOKEN")
		namespace := os.Getenv("OPENBAO_NAMESPACE")

		return NewOpenBaoManager(ctx, address, token, namespace)
	case "local":
		// Local provider doesn't require any external configuration
		return NewLocalManager(ctx)
	case "bitwarden":
		// Bitwarden uses its own organization concept; projectID is treated
		// as the Bitwarden organization ID (or falls back to env vars).
		return NewBitwardenManager(ctx, projectID)
	case "withsecrets":
		// Opt-in hosted vault. Existing configs never select this provider.
		return NewWithSecretsManager()
	default:
		return nil, fmt.Errorf("unsupported cloud provider: %s", provider)
	}
}

// GetSecretsForEnvironment retrieves all secrets and values for a given environment configuration
func (f *SecretManagerFactory) GetSecretsForEnvironment(ctx context.Context, env *config.Environment) (map[string]string, error) {
	values, _, err := f.GetSecretsForEnvironmentWithCache(ctx, env, "", "")
	return values, err
}

// GetSecretsForEnvironmentWithCache retrieves all secrets and values for a given environment configuration with caching.
// The second return value maps env var names to original provider keys for path-expanded entries.
func (f *SecretManagerFactory) GetSecretsForEnvironmentWithCache(ctx context.Context, env *config.Environment, configPath, envName string) (map[string]string, map[string]string, error) {
	logger := log.NewLogger()

	// Initialize cache manager if config path is provided
	var cacheManager *cache.Manager
	var cacheEnabled bool
	var cacheTTL time.Duration

	if configPath != "" {
		// Load global config
		globalConfig, err := config.LoadGlobalConfig()
		if err != nil {
			logger.Debug("Failed to load global config, using defaults", "error", err)
			globalConfig = config.DefaultGlobalConfig()
		}

		// Check if caching should be enabled (global or environment level)
		shouldEnableCache := globalConfig.Cache.Enabled
		if env.Cache != nil {
			shouldEnableCache = env.Cache.Enabled
		}

		// Only create cache manager if caching is enabled
		if shouldEnableCache {
			// Convert to cache types
			cacheGlobalConfig := &cache.GlobalConfig{
				Cache: cache.CacheConfig{
					Enabled: globalConfig.Cache.Enabled,
					TTL:     globalConfig.Cache.TTL,
				},
			}

			cacheManager, err = cache.NewManager(cacheGlobalConfig)
			if err != nil {
				logger.Debug("Failed to initialize cache manager", "error", err)
			} else {
				// Convert env cache config
				var envCache *cache.CacheConfig
				if env.Cache != nil {
					envCache = &cache.CacheConfig{
						Enabled: env.Cache.Enabled,
						TTL:     env.Cache.TTL,
					}
				}

				cacheEnabled, cacheTTL = cacheManager.GetCacheConfig(envCache)
				logger.Debug("Cache configuration", "enabled", cacheEnabled, "ttl", cacheTTL)
			}
		} else {
			logger.Debug("Caching disabled", "global_enabled", globalConfig.Cache.Enabled, "env_cache", env.Cache != nil)
		}
	}

	envItems := env.GetEnvItems()

	// Try to restore a complete cached environment before contacting providers.
	if cacheManager != nil && cacheEnabled && configPath != "" && envName != "" {
		logger.Debug("Attempting to retrieve secrets from cache", "config_path", configPath, "env_name", envName)
		if allSecrets, sourceRefs, ok := tryRestoreCachedEnvironment(logger, cacheManager, env, envItems, configPath, envName); ok {
			logger.Debug("All secrets retrieved from cache", "count", len(allSecrets))
			applyStaticValues(allSecrets, envItems)
			interpolateAll(allSecrets)
			_ = cacheManager.Close()
			return allSecrets, sourceRefs, nil
		}
		logger.Debug("Not all secrets found in cache, fetching from providers")
	}

	// Group mappings by provider/project/region for secret-based mappings
	providerGroups := make(map[fetchKey][]string)

	// Group mappings by provider/project/region for path-based mappings
	pathGroups := make(map[fetchKey]map[string][]string)

	// Group mappings by provider/project/region for parameter-based mappings
	paramGroups := make(map[fetchKey][]string)

	// Group mappings by provider/project/region for parameter path-based mappings
	paramPathGroups := make(map[fetchKey]map[string][]string)

	logger.Debug("Processing environment mappings", "total_mappings", len(envItems))

	// Process all env items to separate secret-based and value-based ones
	for i, envItem := range envItems {
		logger.Debug("Processing mapping", "index", i, "env_var", envItem.EnvironmentVariable, "has_secret_key", envItem.SecretKey != "", "has_secret_path", len(envItem.SecretPath) > 0, "has_param_key", envItem.ParamKey != "", "has_param_path", len(envItem.ParamPath) > 0, "has_value", envItem.Value != nil)

		// Handle direct values first
		if envItem.Value != nil {
			logger.Debug("Skipping secret processing for value-based mapping", "env_var", envItem.EnvironmentVariable)
			continue // Skip secret processing for value-based mappings
		}

		key := resolveFetchKey(env, envItem)

		// Process secret-based mappings (single key)
		if envItem.SecretKey != "" {
			logger.Debug("Adding secret-based mapping to provider group", "provider", key.provider, "project", key.project, "region", key.region, "secret_key", envItem.SecretKey)
			providerGroups[key] = append(providerGroups[key], envItem.SecretKey)
		}

		// Process path-based mappings
		if len(envItem.SecretPath) > 0 {
			logger.Debug("Adding path-based mapping to provider group", "provider", key.provider, "project", key.project, "region", key.region, "secret_path", envItem.SecretPath)
			if pathGroups[key] == nil {
				pathGroups[key] = make(map[string][]string)
			}
			pathGroups[key][envItem.EnvironmentVariable] = envItem.SecretPath
		}

		// Process parameter-based mappings (single key)
		if envItem.ParamKey != "" {
			logger.Debug("Adding param-based mapping to provider group", "provider", key.provider, "project", key.project, "region", key.region, "param_key", envItem.ParamKey)
			paramGroups[key] = append(paramGroups[key], envItem.ParamKey)
		}

		// Process parameter path-based mappings
		if len(envItem.ParamPath) > 0 {
			logger.Debug("Adding param path-based mapping to provider group", "provider", key.provider, "project", key.project, "region", key.region, "param_path", envItem.ParamPath)
			if paramPathGroups[key] == nil {
				paramPathGroups[key] = make(map[string][]string)
			}
			paramPathGroups[key][envItem.EnvironmentVariable] = envItem.ParamPath
		}
	}

	logger.Debug("Provider groups created", "secret_providers", len(providerGroups), "path_providers", len(pathGroups), "param_providers", len(paramGroups), "param_path_providers", len(paramPathGroups))

	// Fetch secrets from each provider
	allSecrets := make(map[string]string)
	sourceRefs := make(map[string]string)
	secretPathCaptures := make(map[string]*capturedPathMapping)
	paramPathCaptures := make(map[string]*capturedPathMapping)

	for key, secretIDs := range providerGroups {
		logger.Debug("Creating secret manager", "provider", key.provider, "project", key.project, "region", key.region, "secret_count", len(secretIDs))

		secretManager, err := f.CreateSecretManager(ctx, key.provider, key.project, key.region)
		if err != nil {
			logger.Debug("Failed to create secret manager", "provider", key.provider, "project", key.project, "region", key.region, "error", err)
			fmt.Printf("Warning: failed to create secret manager for %s: %v\n", key.provider, err)
			continue
		}
		defer secretManager.Close()

		logger.Debug("Fetching secrets from provider", "provider", key.provider, "project", key.project, "region", key.region, "secret_ids", secretIDs)
		secrets, err := secretManager.GetSecrets(key.project, secretIDs)
		if err != nil {
			logger.Debug("Failed to get secrets from provider", "provider", key.provider, "project", key.project, "region", key.region, "error", err)
			fmt.Printf("Warning: failed to get secrets from %s project %s: %v\n", key.provider, key.project, err)
			continue
		}

		logger.Debug("Successfully retrieved secrets from provider", "provider", key.provider, "project", key.project, "region", key.region, "retrieved_count", len(secrets))

		for _, envItem := range envItems {
			if envItem.SecretKey != "" {
				itemKey := resolveFetchKey(env, envItem)
				if itemKey == key {
					if secretValue, exists := secrets[envItem.SecretKey]; exists {
						allSecrets[envItem.EnvironmentVariable] = secretValue
						sourceRefs[envItem.EnvironmentVariable] = envItem.SecretKey
						logger.Debug("Mapped secret to environment variable", "env_var", envItem.EnvironmentVariable, "secret_key", envItem.SecretKey, "provider", key.provider, "project", key.project, "region", key.region)
					} else {
						logger.Debug("Secret key not found in provider response", "env_var", envItem.EnvironmentVariable, "secret_key", envItem.SecretKey, "provider", key.provider, "project", key.project, "region", key.region)
					}
				}
			}
		}
	}

	// Process path-based mappings
	for key, pathMappings := range pathGroups {
		secretManager, err := f.CreateSecretManager(ctx, key.provider, key.project, key.region)
		if err != nil {
			fmt.Printf("Warning: failed to create secret manager for %s: %v\n", key.provider, err)
			continue
		}
		defer secretManager.Close()

		for envVar, secretPaths := range pathMappings {
			id := pathMappingCacheID(pathMappingKindSecret, key, envVar, secretPaths)
			captured := &capturedPathMapping{overlay: make(map[string]resolvedPathEntry), complete: true}
			secretPathCaptures[id] = captured
			for _, secretPath := range secretPaths {
				secrets, err := secretManager.GetSecretsByPath(key.project, secretPath)
				if err != nil {
					fmt.Printf("Warning: failed to get secrets from path '%s': %v\n", secretPath, err)
					captured.complete = false
					continue
				}

				entries := resolvePathEntries(envVar, secretPath, secrets)
				mergeResolvedPathEntries(allSecrets, sourceRefs, entries)
				overlayPathEntries(captured.overlay, entries)
			}
			logger.Debug("Resolved path mapping", "mapping_type", "secret-path", "provider", key.provider, "mapping_key", envVar, "paths", secretPaths, "entries", len(captured.overlay), "complete", captured.complete)
		}
	}

	// Process parameter-based mappings
	for key, paramNames := range paramGroups {
		secretManager, err := f.CreateSecretManager(ctx, key.provider, key.project, key.region)
		if err != nil {
			fmt.Printf("Warning: failed to create secret manager for %s: %v\n", key.provider, err)
			continue
		}
		defer secretManager.Close()

		paramStore, ok := secretManager.(ParameterStore)
		if !ok {
			fmt.Printf("Warning: provider %s does not support Parameter Store mappings\n", key.provider)
			continue
		}

		params, err := paramStore.GetParameters(key.project, paramNames)
		if err != nil {
			fmt.Printf("Warning: failed to get parameters from %s project %s: %v\n", key.provider, key.project, err)
			continue
		}

		for _, envItem := range envItems {
			if envItem.ParamKey != "" {
				itemKey := resolveFetchKey(env, envItem)
				if itemKey == key {
					if paramValue, exists := params[envItem.ParamKey]; exists {
						allSecrets[envItem.EnvironmentVariable] = paramValue
						sourceRefs[envItem.EnvironmentVariable] = envItem.ParamKey
						logger.Debug("Mapped parameter to environment variable", "env_var", envItem.EnvironmentVariable, "param_key", envItem.ParamKey, "provider", key.provider, "project", key.project, "region", key.region)
					}
				}
			}
		}
	}

	// Process parameter path-based mappings
	for key, pathMappings := range paramPathGroups {
		secretManager, err := f.CreateSecretManager(ctx, key.provider, key.project, key.region)
		if err != nil {
			fmt.Printf("Warning: failed to create secret manager for %s: %v\n", key.provider, err)
			continue
		}
		defer secretManager.Close()

		paramStore, ok := secretManager.(ParameterStore)
		if !ok {
			fmt.Printf("Warning: provider %s does not support Parameter Store path mappings\n", key.provider)
			continue
		}

		for envVar, paramPaths := range pathMappings {
			id := pathMappingCacheID(pathMappingKindParam, key, envVar, paramPaths)
			captured := &capturedPathMapping{overlay: make(map[string]resolvedPathEntry), complete: true}
			paramPathCaptures[id] = captured
			for _, paramPath := range paramPaths {
				params, err := paramStore.GetParametersByPath(key.project, paramPath)
				if err != nil {
					fmt.Printf("Warning: failed to get parameters from path '%s': %v\n", paramPath, err)
					captured.complete = false
					continue
				}

				entries := resolvePathEntries(envVar, paramPath, params)
				mergeResolvedPathEntries(allSecrets, sourceRefs, entries)
				overlayPathEntries(captured.overlay, entries)
			}
			logger.Debug("Resolved path mapping", "mapping_type", "param-path", "provider", key.provider, "mapping_key", envVar, "paths", paramPaths, "entries", len(captured.overlay), "complete", captured.complete)
		}
	}

	applyStaticValues(allSecrets, envItems)
	interpolateAll(allSecrets)

	if cacheManager != nil && cacheEnabled && configPath != "" && envName != "" {
		cacheResolvedEnvironment(logger, cacheManager, env, envItems, configPath, envName, cacheTTL, allSecrets, secretPathCaptures, paramPathCaptures)
	}

	// Clean up cache manager
	if cacheManager != nil {
		_ = cacheManager.Close()
	}

	return allSecrets, sourceRefs, nil
}

// fetchKey identifies a provider client instance (provider + project + region).
type fetchKey struct {
	provider string
	project  string
	region   string
}

// resolveFetchKey returns the effective provider, project, and region for an env item.
func resolveFetchKey(env *config.Environment, envItem config.EnvItem) fetchKey {
	provider := envItem.Provider
	if provider == "" {
		provider = env.Provider
	}

	project := envItem.Project
	if project == "" {
		project = env.Project
	}

	region := envItem.Region
	if region == "" {
		region = env.Region
	}

	// For AWS, Azure, OpenBao, Bitwarden, and local, we use a default project key since they don't use projects in the same way as GCP
	if (provider == "aws" || provider == "azure" || provider == "openbao" || provider == "bitwarden" || provider == "local") && project == "" {
		project = "default"
	}

	return fetchKey{provider: provider, project: project, region: region}
}

// injectPathSecrets merges path-fetched secrets into allSecrets.
// secrets is keyed by original provider name; relative env var names are derived from basePath.
// When mappingKey is "*", each relative name is used as the env var directly;
// otherwise names are prefixed with mappingKey + "_". Later calls overlay earlier ones.
func injectPathSecrets(allSecrets, sourceRefs map[string]string, mappingKey, basePath string, secrets map[string]string) {
	mergeResolvedPathEntries(allSecrets, sourceRefs, resolvePathEntries(mappingKey, basePath, secrets))
}
