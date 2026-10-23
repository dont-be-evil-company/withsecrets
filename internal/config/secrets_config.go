package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/dont-be-evil-company/withsecrets/internal/lib/cache"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/log"
	"gopkg.in/yaml.v3"
)

// SecretsConfig represents the structure of a ws.yaml file
type SecretsConfig struct {
	Environments map[string]Environment `yaml:",inline"`
}

// Environment represents a single environment configuration
type Environment struct {
	Provider string             `yaml:"provider"`
	Project  string             `yaml:"project"`
	Region   string             `yaml:"region,omitempty"`
	Env      map[string]EnvItem `yaml:"env"`
	Inherits []string           `yaml:"inherits,omitempty"`
	Cache    *cache.CacheConfig `yaml:"cache,omitempty"`
}

// UnmarshalYAML implements custom YAML unmarshaling for Environment to support
// inherits provided as either a single string or a list of strings.
func (e *Environment) UnmarshalYAML(value *yaml.Node) error {
	type rawEnv struct {
		Provider string             `yaml:"provider"`
		Project  string             `yaml:"project"`
		Region   string             `yaml:"region,omitempty"`
		Env      map[string]EnvItem `yaml:"env"`
		Inherits interface{}        `yaml:"inherits,omitempty"`
		Cache    interface{}        `yaml:"cache,omitempty"`
	}
	var tmp rawEnv
	if err := value.Decode(&tmp); err != nil {
		return err
	}
	e.Provider = tmp.Provider
	e.Project = tmp.Project
	e.Region = tmp.Region
	e.Env = tmp.Env

	// Normalize inherits to []string
	e.Inherits = nil
	switch v := tmp.Inherits.(type) {
	case nil:
		// nothing
	case string:
		if v != "" {
			e.Inherits = []string{v}
		}
	case []interface{}:
		list := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				list = append(list, s)
			}
		}
		if len(list) > 0 {
			e.Inherits = list
		}
	default:
		return fmt.Errorf("invalid type for inherits: %T", v)
	}

	// Parse cache configuration
	e.Cache = nil
	if tmp.Cache != nil {
		cacheConfig := &cache.CacheConfig{}
		duration, enabled, err := cache.ParseDuration(tmp.Cache)
		if err != nil {
			return fmt.Errorf("failed to parse cache configuration: %w", err)
		}
		cacheConfig.Enabled = enabled
		cacheConfig.TTL = duration
		e.Cache = cacheConfig
	}

	return nil
}

// EnvItem represents an environment variable configuration in the new simplified format
// It can be either a string (just the env var name) or a full mapping object
type EnvItem struct {
	// For string format: just the environment variable name
	EnvironmentVariable string   `yaml:"environment-variable,omitempty"`
	SecretKey           string   `yaml:"secret-key,omitempty"`
	Field               string   `yaml:"field,omitempty"`
	SecretPath          []string `yaml:"secret-path,omitempty"`
	ParamKey            string   `yaml:"param-key,omitempty"`
	ParamPath           []string `yaml:"param-path,omitempty"`
	Value               any      `yaml:"value,omitempty"`
	Provider            string   `yaml:"provider,omitempty"`
	Project             string   `yaml:"project,omitempty"`
	Region              string   `yaml:"region,omitempty"`
}

// UnmarshalYAML implements custom YAML unmarshaling for EnvItem
// This allows it to handle both string format (just env var name) and object format
func (e *EnvItem) UnmarshalYAML(value *yaml.Node) error {
	// For map syntax, the env var name is the map key; object holds fields only
	var temp struct {
		SecretKey  string `yaml:"secret-key,omitempty"`
		Field      string `yaml:"field,omitempty"`
		SecretPath any    `yaml:"secret-path,omitempty"`
		ParamKey   string `yaml:"param-key,omitempty"`
		ParamPath  any    `yaml:"param-path,omitempty"`
		Value      any    `yaml:"value,omitempty"`
		Provider   string `yaml:"provider,omitempty"`
		Project    string `yaml:"project,omitempty"`
		Region     string `yaml:"region,omitempty"`
	}
	if err := value.Decode(&temp); err != nil {
		return err
	}
	secretPath, err := decodeStringOrList(temp.SecretPath)
	if err != nil {
		return fmt.Errorf("secret-path: %w", err)
	}
	paramPath, err := decodeStringOrList(temp.ParamPath)
	if err != nil {
		return fmt.Errorf("param-path: %w", err)
	}
	e.SecretKey = temp.SecretKey
	e.Field = temp.Field
	e.SecretPath = secretPath
	e.ParamKey = temp.ParamKey
	e.ParamPath = paramPath
	e.Value = temp.Value
	e.Provider = temp.Provider
	e.Project = temp.Project
	e.Region = temp.Region
	return nil
}

// decodeStringOrList normalizes a YAML string or list of strings into []string.
// Empty strings are skipped; an empty result is treated as unset (nil).
func decodeStringOrList(v any) ([]string, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case string:
		if strings.TrimSpace(x) == "" {
			return nil, nil
		}
		return []string{x}, nil
	case []string:
		out := make([]string, 0, len(x))
		for _, s := range x {
			if t := strings.TrimSpace(s); t != "" {
				out = append(out, t)
			}
		}
		if len(out) == 0 {
			return nil, nil
		}
		return out, nil
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("invalid type in list: %T", item)
			}
			if t := strings.TrimSpace(s); t != "" {
				out = append(out, t)
			}
		}
		if len(out) == 0 {
			return nil, nil
		}
		return out, nil
	default:
		return nil, fmt.Errorf("must be a string or list of strings, got %T", v)
	}
}

func interpolatePaths(paths []string, resolvedVars map[string]string) []string {
	if len(paths) == 0 {
		return paths
	}
	out := make([]string, len(paths))
	for i, p := range paths {
		if strings.Contains(p, "${") {
			out[i] = InterpolateEnvVars(p, resolvedVars)
		} else {
			out[i] = p
		}
	}
	return out
}

// GetEnvItems returns all env items for an environment
func (e *Environment) GetEnvItems() []EnvItem {
	items := make([]EnvItem, 0, len(e.Env))
	for name, item := range e.Env {
		item.EnvironmentVariable = name
		items = append(items, item)
	}
	return items
}

// InterpolateEnvVars replaces ${VAR_NAME} patterns with actual environment variable values
// It also supports previously resolved variables from the same configuration
// Supports both ${VAR_NAME} and ${VAR_NAME:-default} syntax
func InterpolateEnvVars(value string, resolvedVars map[string]string) string {
	// Regex to match ${VAR_NAME} and ${VAR_NAME:-default} patterns
	re := regexp.MustCompile(`\$\{([^}]+)\}`)

	// Keep interpolating until no more changes are made to handle nested variables
	prevValue := ""
	for prevValue != value {
		prevValue = value
		value = re.ReplaceAllStringFunc(value, func(match string) string {
			// Extract the variable name and optional default from ${VAR_NAME} or ${VAR_NAME:-default}
			content := match[2 : len(match)-1]

			// Check if there's a default value specified
			if strings.Contains(content, ":-") {
				parts := strings.SplitN(content, ":-", 2)
				varName := parts[0]
				defaultValue := parts[1]

				// First check if we have this variable from previously resolved mappings
				if resolvedValue, exists := resolvedVars[varName]; exists {
					return resolvedValue
				}

				// Then check if it's an environment variable
				if envValue := os.Getenv(varName); envValue != "" {
					return envValue
				}

				// If not found, return the default value
				return defaultValue
			}

			// No default value specified, use original logic
			varName := content

			// First check if we have this variable from previously resolved mappings
			if resolvedValue, exists := resolvedVars[varName]; exists {
				return resolvedValue
			}

			// Then check if it's an environment variable
			if envValue := os.Getenv(varName); envValue != "" {
				return envValue
			}

			// If not found, return the original pattern (could be useful for debugging)
			return match
		})
	}

	return value
}

// interpolateConfigVars replaces ${VAR_NAME} patterns with resolved variables from configuration only
// This version does not check system environment variables, only the resolvedVars map
func interpolateConfigVars(value string, resolvedVars map[string]string) string {
	// Regex to match ${VAR_NAME} and ${VAR_NAME:-default} patterns
	re := regexp.MustCompile(`\$\{([^}]+)\}`)

	// Keep interpolating until no more changes are made to handle nested variables
	prevValue := ""
	for prevValue != value {
		prevValue = value
		value = re.ReplaceAllStringFunc(value, func(match string) string {
			// Extract the variable name and optional default from ${VAR_NAME} or ${VAR_NAME:-default}
			content := match[2 : len(match)-1]

			// Check if there's a default value specified
			if strings.Contains(content, ":-") {
				parts := strings.SplitN(content, ":-", 2)
				varName := parts[0]
				defaultValue := parts[1]

				// Check if we have this variable from previously resolved mappings
				if resolvedValue, exists := resolvedVars[varName]; exists {
					return resolvedValue
				}

				// If not found, return the default value
				return defaultValue
			}

			// No default value specified, use original logic
			varName := content

			// Check if we have this variable from previously resolved mappings
			if resolvedValue, exists := resolvedVars[varName]; exists {
				return resolvedValue
			}

			// If not found, return the original pattern (could be useful for debugging)
			return match
		})
	}

	return value
}

// processValueInterpolations processes all value fields in env items to resolve environment variable interpolations
func processValueInterpolations(config *SecretsConfig) error {
	// Process environments in order to handle dependencies correctly
	// We'll process each environment multiple times until no more interpolations are possible
	// or until we detect a circular dependency

	for envName, env := range config.Environments {
		// Track resolved variables for this environment
		resolvedVars := make(map[string]string)

		// Process env items multiple times to handle dependencies
		maxIterations := len(env.Env) * 2 // Allow for some dependency depth
		for iteration := 0; iteration < maxIterations; iteration++ {
			changed := false

			// Process env items
			for name, envItem := range env.Env {
				if envItem.Value != nil {
					// Convert value to string for processing
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

					// Check if this value contains interpolation patterns
					if strings.Contains(strValue, "${") {
						// Interpolate the value
						interpolatedValue := InterpolateEnvVars(strValue, resolvedVars)

						// If the value changed, update it
						if interpolatedValue != strValue {
							// Update the env item value
							tmp := env.Env[name]
							tmp.Value = interpolatedValue
							env.Env[name] = tmp
							// Update our resolved vars map
							resolvedVars[name] = interpolatedValue
							changed = true
						}
					} else {
						// No interpolation needed, but convert numeric values to strings for consistency
						if envItem.Value != strValue {
							tmp := env.Env[name]
							tmp.Value = strValue
							env.Env[name] = tmp
							changed = true
						}
						// Store the value in resolved vars and trigger another iteration
						// only if this introduces a new mapping or changes an existing one.
						prev, exists := resolvedVars[name]
						resolvedVars[name] = strValue
						if !exists || prev != strValue {
							changed = true
						}
					}
				}
			}

			// If no changes were made in this iteration, we're done
			if !changed {
				break
			}
		}

		// After env item values are processed, interpolate other string fields that may reference them
		// Build a map of resolved variables from the now-updated env values
		resolvedVars = make(map[string]string)
		for name, envItem := range env.Env {
			if envItem.Value != nil {
				resolvedVars[name] = fmt.Sprintf("%v", envItem.Value)
			}
		}

		// Interpolate environment-level project field
		if env.Project != "" && strings.Contains(env.Project, "${") {
			interpolated := InterpolateEnvVars(env.Project, resolvedVars)
			if interpolated != env.Project {
				env.Project = interpolated
			}
		}

		// Interpolate environment-level region field
		if env.Region != "" && strings.Contains(env.Region, "${") {
			interpolated := InterpolateEnvVars(env.Region, resolvedVars)
			if interpolated != env.Region {
				env.Region = interpolated
			}
		}

		// Interpolate item-level fields that can be strings
		for name, envItem := range env.Env {
			// secret-key
			if envItem.SecretKey != "" && strings.Contains(envItem.SecretKey, "${") {
				envItem.SecretKey = InterpolateEnvVars(envItem.SecretKey, resolvedVars)
			}
			if envItem.Field != "" && strings.Contains(envItem.Field, "${") {
				envItem.Field = InterpolateEnvVars(envItem.Field, resolvedVars)
			}
			// secret-path
			if len(envItem.SecretPath) > 0 {
				envItem.SecretPath = interpolatePaths(envItem.SecretPath, resolvedVars)
			}
			// param-key
			if envItem.ParamKey != "" && strings.Contains(envItem.ParamKey, "${") {
				envItem.ParamKey = InterpolateEnvVars(envItem.ParamKey, resolvedVars)
			}
			// param-path
			if len(envItem.ParamPath) > 0 {
				envItem.ParamPath = interpolatePaths(envItem.ParamPath, resolvedVars)
			}
			// project (item-level)
			if envItem.Project != "" && strings.Contains(envItem.Project, "${") {
				envItem.Project = InterpolateEnvVars(envItem.Project, resolvedVars)
			}
			// region (item-level)
			if envItem.Region != "" && strings.Contains(envItem.Region, "${") {
				envItem.Region = InterpolateEnvVars(envItem.Region, resolvedVars)
			}
			env.Env[name] = envItem
		}

		// Update the environment in the config
		config.Environments[envName] = env
	}

	return nil
}

// resolveInheritance merges env variables from inherited environments into each environment.
// Inheritance is processed in order; later entries in "inherits" override earlier ones only
// if the current environment does not provide an explicit override. Current environment values
// always take precedence over inherited ones. Cycles are detected and reported.
func resolveInheritance(config *SecretsConfig) error {
	// Memoize resolved env maps to avoid re-computation
	resolved := make(map[string]map[string]EnvItem)
	resolving := make(map[string]bool)

	var resolveEnv func(name string) (map[string]EnvItem, error)
	resolveEnv = func(name string) (map[string]EnvItem, error) {
		if env, ok := resolved[name]; ok {
			return env, nil
		}
		if resolving[name] {
			return nil, fmt.Errorf("inheritance cycle detected involving environment '%s'", name)
		}
		base, ok := config.Environments[name]
		if !ok {
			return nil, fmt.Errorf("inherits references unknown environment '%s'", name)
		}
		resolving[name] = true

		// Start with an empty map, merge inherited in order
		merged := make(map[string]EnvItem)
		for _, parentName := range base.Inherits {
			parentEnv, err := resolveEnv(parentName)
			if err != nil {
				return nil, err
			}
			// Merge from parent; do not overwrite existing keys
			for k, v := range parentEnv {
				if _, exists := merged[k]; !exists {
					merged[k] = v
				}
			}
		}

		// Finally, overlay current environment's own variables (override parents)
		for k, v := range base.Env {
			merged[k] = v
		}

		resolving[name] = false
		resolved[name] = merged
		return merged, nil
	}

	// Resolve for all environments and write back the merged maps
	for envName := range config.Environments {
		merged, err := resolveEnv(envName)
		if err != nil {
			return err
		}
		env := config.Environments[envName]
		env.Env = merged
		config.Environments[envName] = env
	}
	return nil
}

// LoadSecretsConfig loads the ws.yaml configuration file
func LoadSecretsConfig(configPath string) (*SecretsConfig, error) {
	logger := log.NewLogger()

	if configPath == "" {
		configPath = DefaultConfigFileName
	}

	logger.Debug("Loading configuration file", "path", configPath)

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		logger.Debug("Configuration file not found", "path", configPath)
		return nil, fmt.Errorf("configuration file not found: %s", configPath)
	}

	// Read file
	logger.Debug("Reading configuration file")
	data, err := os.ReadFile(configPath)
	if err != nil {
		logger.Debug("Failed to read configuration file", "path", configPath, "error", err)
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	logger.Debug("Configuration file read successfully", "size_bytes", len(data))

	// Parse YAML
	logger.Debug("Parsing YAML configuration")
	var config SecretsConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		logger.Debug("Failed to parse YAML configuration", "error", err)
		return nil, fmt.Errorf("failed to parse configuration file: %w", err)
	}

	logger.Debug("YAML parsed successfully", "environments_count", len(config.Environments))

	// Resolve inheritance before any interpolations or validation
	logger.Debug("Resolving environment inheritance")
	if err := resolveInheritance(&config); err != nil {
		logger.Debug("Failed to resolve environment inheritance", "error", err)
		return nil, fmt.Errorf("failed to resolve inheritance: %w", err)
	}
	logger.Debug("Environment inheritance resolved successfully")

	// Process environment variable interpolations
	logger.Debug("Processing environment variable interpolations")
	if err := processValueInterpolations(&config); err != nil {
		logger.Debug("Failed to process environment variable interpolations", "error", err)
		return nil, fmt.Errorf("failed to process environment variable interpolations: %w", err)
	}
	logger.Debug("Environment variable interpolations processed successfully")

	// Validate configuration
	logger.Debug("Validating configuration")
	if err := validateConfig(&config); err != nil {
		logger.Debug("Configuration validation failed", "error", err)
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	logger.Debug("Configuration validation passed")
	return &config, nil
}

// GetEnvironment returns the configuration for a specific environment
func (c *SecretsConfig) GetEnvironment(envName string) (*Environment, error) {
	logger := log.NewLogger()

	if envName == "" {
		envName = "default"
		logger.Debug("No environment specified, using default")
	}

	logger.Debug("Getting environment configuration", "requested_env", envName, "available_environments", len(c.Environments))

	env, exists := c.Environments[envName]
	if !exists {
		logger.Debug("Environment not found in configuration", "requested_env", envName, "available_environments", getEnvironmentNames(c.Environments))
		return nil, fmt.Errorf("environment '%s' not found in configuration", envName)
	}

	logger.Debug("Environment configuration retrieved", "environment", envName, "provider", env.Provider, "project", env.Project, "env_count", len(env.Env))
	return &env, nil
}

// getEnvironmentNames returns a slice of available environment names
func getEnvironmentNames(environments map[string]Environment) []string {
	names := make([]string, 0, len(environments))
	for name := range environments {
		names = append(names, name)
	}
	return names
}

// validateConfig validates the configuration structure
func validateConfig(config *SecretsConfig) error {
	if len(config.Environments) == 0 {
		return fmt.Errorf("no environments defined in configuration")
	}

	for envName, env := range config.Environments {
		if env.Provider == "" {
			return fmt.Errorf("environment '%s': provider is required", envName)
		}

		// Project is required for all providers except AWS, Azure, OpenBao, Bitwarden, and local
		if env.Project == "" && env.Provider != "aws" && env.Provider != "azure" && env.Provider != "openbao" && env.Provider != "bitwarden" && env.Provider != "local" {
			return fmt.Errorf("environment '%s': project is required for provider '%s'", envName, env.Provider)
		}

		// At least one env item must be provided, possibly via inheritance
		if len(env.Env) == 0 {
			return fmt.Errorf("environment '%s': at least one env item is required (directly or via inherits)", envName)
		}

		// Validate env items
		idx := 0
		for envVarName, envItem := range env.Env {
			idx++
			// name is the environment variable

			// Exactly one of secret-key, secret-path, param-key, param-path, or value
			secretFields := 0
			if envItem.SecretKey != "" {
				secretFields++
			}
			if len(envItem.SecretPath) > 0 {
				secretFields++
			}
			if envItem.ParamKey != "" {
				secretFields++
			}
			if len(envItem.ParamPath) > 0 {
				secretFields++
			}
			if envItem.Value != nil {
				secretFields++
			}

			if secretFields == 0 {
				return fmt.Errorf("environment '%s': env item %d: either secret-key, secret-path, param-key, param-path, or value is required", envName, idx)
			}

			if secretFields > 1 {
				return fmt.Errorf("environment '%s': env item %d: cannot specify multiple of secret-key, secret-path, param-key, param-path, or value", envName, idx)
			}

			// "*" is only valid with path-based bulk loading
			if envVarName == "*" {
				if len(envItem.SecretPath) == 0 && len(envItem.ParamPath) == 0 {
					return fmt.Errorf("environment '%s': env item '*': must use secret-path or param-path", envName)
				}
			}

			// Determine effective provider for this item
			effectiveProvider := env.Provider
			if envItem.Provider != "" {
				effectiveProvider = envItem.Provider
			}

			// Validate provider value if set on item
			if envItem.Provider != "" && !isValidProvider(envItem.Provider) {
				return fmt.Errorf("environment '%s': env item %d: invalid provider '%s'", envName, idx, envItem.Provider)
			}

			// param-key / param-path are AWS Parameter Store only
			if (envItem.ParamKey != "" || len(envItem.ParamPath) > 0) && effectiveProvider != "aws" {
				return fmt.Errorf("environment '%s': env item %d: param-key and param-path are only supported with provider 'aws'", envName, idx)
			}

			// field selects one key inside an OpenBao secret
			if envItem.Field != "" {
				if effectiveProvider != "openbao" {
					return fmt.Errorf("environment '%s': env item %d: field is only supported with provider 'openbao'", envName, idx)
				}
				if envItem.SecretKey == "" {
					return fmt.Errorf("environment '%s': env item %d: field requires secret-key", envName, idx)
				}
			}

			// Local provider rules: only value is allowed
			if effectiveProvider == "local" {
				if envItem.Value == nil {
					return fmt.Errorf("environment '%s': env item %d: provider 'local' requires 'value'", envName, idx)
				}
				if envItem.SecretKey != "" || len(envItem.SecretPath) > 0 || envItem.ParamKey != "" || len(envItem.ParamPath) > 0 {
					return fmt.Errorf("environment '%s': env item %d: provider 'local' does not support 'secret-key', 'secret-path', 'param-key', or 'param-path'", envName, idx)
				}
			}
		}

		// Validate main provider
		if !isValidProvider(env.Provider) {
			return fmt.Errorf("environment '%s': invalid provider '%s'", envName, env.Provider)
		}
	}

	return nil
}

// isValidProvider checks if the provider is supported
func isValidProvider(provider string) bool {
	validProviders := []string{"gcp", "aws", "azure", "openbao", "bitwarden", "local"}
	for _, p := range validProviders {
		if p == provider {
			return true
		}
	}
	return false
}
