package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/log"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/secrets"
	"github.com/spf13/cobra"
)

var (
	showEnvironment string
	showConfigFile  string
	showSensitive   bool
	showOutput      string
)

const (
	showListEnvironmentsValue = "__LIST_ENVIRONMENTS__"
)

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Show environment variables from ws.yaml",
	Long: `Show environment variables from ws.yaml configuration.

This command displays environment variables in KEY=value format, similar to
'ws run --contain -- env', but only includes values from ws.yaml.

You can filter the output by providing one or more pattern arguments. Patterns
are case-insensitive and support '*' as a wildcard character.

Examples:
  ws show                    # Show all variables from default environment
  ws show db_password        # Show only DB_PASSWORD
  ws show --env staging db*  # Show all variables starting with DB from staging
  ws show db*p*              # Show variables matching DB*P* pattern
  ws show db_* gcp_*         # Show variables starting with DB_ or GCP_
  ws show --sensitive        # Show all variables with redacted values`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		envFlag := cmd.Flags().Lookup("env")
		listEnvironments := envFlag != nil && envFlag.Changed && showEnvironment == showListEnvironmentsValue

		// Allow form: --env <value> (when NoOptDefVal is set) by consuming
		// the next positional argument as the environment if provided.
		if listEnvironments && len(args) > 0 && args[0] != "" {
			showEnvironment = args[0]
			args = args[1:]
			listEnvironments = false
		}

		return runShowCommand(args, listEnvironments)
	},
}

func init() {
	showCmd.Flags().StringVarP(&showEnvironment, "env", "e", "default", "Environment to use (default: default). Provide without value to list available environments.")
	showCmd.Flags().StringVarP(&showConfigFile, "config", "c", "", "Path to ws.yaml configuration file")
	showCmd.Flags().BoolVar(&showSensitive, "sensitive", false, "Redact sensitive values")
	showCmd.Flags().StringVarP(&showOutput, "output", "o", "dotenv", "Output format: dotenv (default), json, shell")
	envFlag := showCmd.Flags().Lookup("env")
	if envFlag != nil {
		envFlag.NoOptDefVal = showListEnvironmentsValue
	}
	rootCmd.AddCommand(showCmd)
}

func runShowCommand(patterns []string, listEnvironments bool) error {
	logger := log.NewLogger()

	if listEnvironments && len(patterns) > 0 && patterns[0] != "" {
		showEnvironment = patterns[0]
		patterns = patterns[1:]
		listEnvironments = false
	}

	// Find configuration file if not specified
	if showConfigFile == "" {
		var err error
		logger.Debug("No config file specified, searching for ws.yaml")
		showConfigFile, err = config.FindConfigFile()
		if err != nil {
			return fmt.Errorf("failed to find configuration file: %w", err)
		}
		logger.Debug("Found configuration file", "path", showConfigFile)
	} else {
		logger.Debug("Using specified configuration file", "path", showConfigFile)
	}

	// Load configuration
	logger.Debug("Loading configuration from file")
	kubaConfig, err := config.LoadSecretsConfig(showConfigFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	logger.Debug("Configuration loaded successfully")

	if listEnvironments {
		logger.Debug("Listing available environments from configuration")
		for _, envName := range getSortedEnvironmentNames(kubaConfig) {
			fmt.Println(envName)
		}
		return nil
	}

	// Get environment configuration
	logger.Debug("Getting environment configuration", "environment", showEnvironment)
	env, err := kubaConfig.GetEnvironment(showEnvironment)
	if err != nil {
		return fmt.Errorf("failed to get environment '%s': %w", showEnvironment, err)
	}
	logger.Debug("Environment configuration retrieved", "environment", showEnvironment, "provider", env.Provider, "env_count", len(env.Env))

	// Create secrets manager factory
	logger.Debug("Creating secrets manager factory")
	factory := secrets.NewSecretManagerFactory()

	// Get secrets for the environment
	ctx := context.Background()
	logger.Debug("Fetching secrets from cloud providers")
	secrets, _, err := factory.GetSecretsForEnvironmentWithCache(ctx, env, showConfigFile, showEnvironment)
	if err != nil {
		return fmt.Errorf("failed to get secrets: %w", err)
	}
	logger.Debug("Secrets retrieved successfully", "count", len(secrets))

	// Filter secrets based on patterns
	filteredSecrets := filterSecrets(secrets, patterns)
	logger.Debug("Filtered secrets", "original_count", len(secrets), "filtered_count", len(filteredSecrets))

	// Prepare secrets for output
	displaySecrets := make(map[string]string, len(filteredSecrets))
	for key, value := range filteredSecrets {
		displayValue := value
		if showSensitive {
			displayValue = maskSecret(value)
		}
		displaySecrets[key] = displayValue
	}

	switch showOutput {
	case "dotenv":
		for _, key := range getSortedKeys(displaySecrets) {
			fmt.Printf("%s=%s\n", key, displaySecrets[key])
		}
	case "shell":
		for _, key := range getSortedKeys(displaySecrets) {
			fmt.Printf("export %s=%s\n", key, displaySecrets[key])
		}
	case "json":
		payload, err := json.MarshalIndent(displaySecrets, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to format secrets as json: %w", err)
		}
		fmt.Println(string(payload))
	default:
		return fmt.Errorf("invalid output format '%s': must be one of: dotenv, json, shell", showOutput)
	}

	return nil
}

// filterSecrets filters a map of secrets based on provided patterns
// Patterns are case-insensitive and support '*' as a wildcard
// If no patterns are provided, all secrets are returned
func filterSecrets(secrets map[string]string, patterns []string) map[string]string {
	if len(patterns) == 0 {
		return secrets
	}

	// Convert patterns to regex patterns
	regexPatterns := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		// Convert wildcard pattern to regex
		// Escape special regex characters except *
		escaped := regexp.QuoteMeta(pattern)
		// Replace escaped \* with .* for wildcard matching
		escaped = strings.ReplaceAll(escaped, "\\*", ".*")
		// Make it case-insensitive by adding (?i) prefix
		regexPattern := "(?i)^" + escaped + "$"
		re, err := regexp.Compile(regexPattern)
		if err != nil {
			// If pattern is invalid, skip it
			continue
		}
		regexPatterns = append(regexPatterns, re)
	}

	// Filter secrets
	filtered := make(map[string]string)
	for key, value := range secrets {
		// Check if key matches any pattern
		for _, re := range regexPatterns {
			if re.MatchString(key) {
				filtered[key] = value
				break // Match found, no need to check other patterns
			}
		}
	}

	return filtered
}

func getSortedEnvironmentNames(cfg *config.SecretsConfig) []string {
	names := make([]string, 0, len(cfg.Environments))
	for name := range cfg.Environments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func getSortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
