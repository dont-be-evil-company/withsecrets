package ws

import (
	"context"
	"fmt"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/log"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/secrets"
	"github.com/spf13/cobra"
)

var (
	testEnvironment string
	testConfigFile  string
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Test secret retrieval for an environment",
	Long: `Test authorization and retrieve all mapped values for the selected environment.

This command will:
1. Locate and load the ws.yaml configuration
2. Resolve the selected environment
3. Test authorization for each provider used in the environment
4. Attempt to fetch all mapped values (secrets, paths, and literals)

It provides clear feedback about authentication status and permissions for each provider.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTest()
	},
}

func init() {
	testCmd.Flags().StringVarP(&testEnvironment, "env", "e", "default", "Environment to use (default: default)")
	testCmd.Flags().StringVarP(&testConfigFile, "config", "c", "", "Path to ws.yaml configuration file")
	rootCmd.AddCommand(testCmd)
}

func runTest() error {
	logger := log.NewLogger()

	// Find configuration file if not specified
	cfgPath := testConfigFile
	if cfgPath == "" {
		logger.Debug("No config file specified, searching for ws.yaml")
		path, err := config.FindConfigFile()
		if err != nil {
			return fmt.Errorf("failed to find configuration file: %w", err)
		}
		cfgPath = path
		logger.Debug("Found configuration file", "path", cfgPath)
	} else {
		logger.Debug("Using specified configuration file", "path", cfgPath)
	}

	// Load configuration
	logger.Debug("Loading configuration from file")
	kubaConfig, err := config.LoadSecretsConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	logger.Debug("Configuration loaded successfully")

	// Get environment configuration
	logger.Debug("Getting environment configuration", "environment", testEnvironment)
	env, err := kubaConfig.GetEnvironment(testEnvironment)
	if err != nil {
		return fmt.Errorf("failed to get environment '%s': %w", testEnvironment, err)
	}

	// Create secrets manager factory
	logger.Debug("Creating secrets manager factory")
	factory := secrets.NewSecretManagerFactory()
	ctx := context.Background()

	// Step 1: Test authorization for all providers used in this environment
	fmt.Printf("\n=== Testing Authorization ===\n\n")

	// Collect unique providers from the environment
	providers := make(map[string]string) // provider -> projectID
	providers[env.Provider] = env.Project

	// Also check item-level providers
	envItems := env.GetEnvItems()
	for _, item := range envItems {
		if item.Provider != "" {
			project := item.Project
			if project == "" {
				project = env.Project
			}
			providers[item.Provider] = project
		}
	}

	// Test authorization for each provider
	authResults := make(map[string]*secrets.AuthorizationTestResult)
	allAuthPassed := true

	for provider, projectID := range providers {
		fmt.Printf("Testing %s provider", provider)
		if projectID != "" {
			fmt.Printf(" (project: %s)", projectID)
		}
		fmt.Printf("...\n")

		result, err := factory.TestAuthorization(ctx, provider, projectID)
		if err != nil {
			fmt.Printf("  ❌ Error testing authorization: %v\n\n", err)
			allAuthPassed = false
			continue
		}

		authResults[provider] = result

		// Print results
		if !result.Authenticated {
			fmt.Printf("  ❌ Authentication failed\n")
			fmt.Printf("     %s\n", result.CredentialsInfo)
			if result.ErrorMessage != "" {
				fmt.Printf("     Error: %s\n", result.ErrorMessage)
			}
			allAuthPassed = false
		} else if !result.HasPermissions {
			fmt.Printf("  ⚠️  Authenticated but lacks permissions\n")
			fmt.Printf("     %s\n", result.CredentialsInfo)
			if result.ErrorMessage != "" {
				fmt.Printf("     Error: %s\n", result.ErrorMessage)
			}
			allAuthPassed = false
		} else {
			fmt.Printf("  ✅ Successfully authenticated and authorized\n")
			fmt.Printf("     %s\n", result.CredentialsInfo)
		}
		fmt.Printf("\n")
	}

	// If authorization failed, provide helpful message but continue to test retrieval
	if !allAuthPassed {
		fmt.Printf("⚠️  Some authorization tests failed. Attempting secret retrieval anyway...\n\n")
	}

	// Step 2: Attempt to retrieve secrets
	fmt.Printf("=== Testing Secret Retrieval ===\n\n")
	logger.Debug("Fetching secrets and values for environment")
	values, _, err := factory.GetSecretsForEnvironmentWithCache(ctx, env, cfgPath, testEnvironment)
	if err != nil {
		return fmt.Errorf("failed to retrieve values: %w", err)
	}

	// Success summary
	fmt.Printf("✅ Successfully retrieved %d values for environment '%s'\n", len(values), testEnvironment)

	// If authorization failed, remind user
	if !allAuthPassed {
		fmt.Printf("\n⚠️  Note: Some authorization tests failed. Please check your credentials and permissions.\n")
	}

	return nil
}
