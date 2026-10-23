package ws

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/apprunner"
	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/log"
	"github.com/spf13/cobra"
	"google.golang.org/api/option"
	run "google.golang.org/api/run/v1"
	"gopkg.in/yaml.v3"
)

var (
	convertFrom     string
	convertEnv      string
	convertInfile   string
	convertOutfile  string
	convertProvider string
	convertProject  string
	convertName     string
)

var convertCmd = &cobra.Command{
	Use:   "convert",
	Short: "Convert configuration from other formats to ws.yaml",
	Long: `Convert configuration files from other formats (e.g., dotenv, ksvc) to ws.yaml format.

This command helps migrate existing configurations to ws.yaml format.
For dotenv files, it will create environment variable entries using the 'value' field.
For ksvc files, it will convert container env definitions (including secret refs)
into ws.yaml env mappings.

Note: When updating an existing ws.yaml file, comments within the modified
environment section will be lost as the section is regenerated. This is a limitation
of YAML manipulation - to preserve structure and data, comments in modified sections
cannot be retained. Consider backing up your ws.yaml file before conversion if
comments are important.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConvert()
	},
}

func init() {
	convertCmd.Flags().StringVar(&convertFrom, "from", "", "Source format (e.g., 'dotenv', 'ksvc')")
	convertCmd.Flags().StringVarP(&convertEnv, "env", "e", "default", "Environment name to use in ws.yaml (default: default)")
	convertCmd.Flags().StringVar(&convertInfile, "infile", "", "Input file path (e.g., .env.example)")
	convertCmd.Flags().StringVar(&convertOutfile, "outfile", "", "Output ws.yaml file path (default: ws.yaml in current directory)")
	convertCmd.Flags().StringVar(&convertProvider, "provider", "", "Provider to read Knative Service from when using --from ksvc without --infile (gcp, aws, azure)")
	convertCmd.Flags().StringVar(&convertProject, "project", "", "Project/namespace to read Knative Service from when using --from ksvc")
	convertCmd.Flags().StringVar(&convertName, "name", "", "Service name to read Knative Service from when using --from ksvc")

	convertCmd.MarkFlagRequired("from")

	rootCmd.AddCommand(convertCmd)
}

func runConvert() error {
	logger := log.NewLogger()

	if convertFrom != "dotenv" && convertFrom != "ksvc" {
		return fmt.Errorf("unsupported source format: %s (supported: 'dotenv', 'ksvc')", convertFrom)
	}

	// Validate input source flags
	switch convertFrom {
	case "dotenv":
		if strings.TrimSpace(convertInfile) == "" {
			return fmt.Errorf("--infile is required when --from is 'dotenv'")
		}
	case "ksvc":
		infile := strings.TrimSpace(convertInfile)
		project := strings.TrimSpace(convertProject)
		name := strings.TrimSpace(convertName)
		provider := strings.TrimSpace(convertProvider)

		if infile == "" {
			if provider == "" {
				return fmt.Errorf("--provider is required when --from 'ksvc' and --infile is omitted")
			}
			if project == "" || name == "" {
				return fmt.Errorf("for --from 'ksvc' without --infile, both --project and --name must be set")
			}
		}
	}

	// Determine output file path
	outPath := convertOutfile
	if outPath == "" {
		outPath = "ws.yaml"
	}

	logger.Debug("Converting configuration to ws.yaml", "infile", convertInfile, "outfile", outPath, "env", convertEnv, "from", convertFrom)

	// Read and parse input file based on source format
	var (
		newEnvItems     map[string]config.EnvItem
		sourceVarsCount int
		defaultProvider string
		defaultProject  string
	)

	switch convertFrom {
	case "dotenv":
		logger.Debug("Reading dotenv file", "path", convertInfile)
		envVars, err := parseDotenvFile(convertInfile)
		if err != nil {
			return fmt.Errorf("failed to parse dotenv file: %w", err)
		}
		logger.Debug("Parsed dotenv file", "variables_count", len(envVars))

		newEnvItems = make(map[string]config.EnvItem, len(envVars))
		for key, value := range envVars {
			// Skip empty values
			if strings.TrimSpace(value) == "" {
				logger.Debug("Skipping empty environment variable", "key", key)
				continue
			}
			newEnvItems[key] = config.EnvItem{
				Value: value,
			}
		}
		sourceVarsCount = len(newEnvItems)
		// For dotenv we default to local provider
		defaultProvider = "local"
		defaultProject = ""
	case "ksvc":
		var (
			items    map[string]config.EnvItem
			provider string
			project  string
			err      error
		)

		if strings.TrimSpace(convertInfile) != "" {
			logger.Debug("Reading ksvc file from local infile", "path", convertInfile)
			items, provider, project, err = parseKsvcFile(convertInfile)
		} else {
			logger.Debug("Reading ksvc manifest from provider API", "provider", convertProvider, "project", convertProject, "name", convertName)
			items, provider, project, err = loadKsvcFromProvider(convertProvider, convertProject, convertName)
		}
		if err != nil {
			return fmt.Errorf("failed to parse ksvc file: %w", err)
		}
		logger.Debug("Parsed ksvc file", "variables_count", len(items), "provider", provider, "project", project)

		newEnvItems = items
		sourceVarsCount = len(newEnvItems)
		// For ksvc we default to gcp provider with project/namespace if present
		if provider == "" {
			provider = "gcp"
		}
		defaultProvider = provider
		defaultProject = project
	default:
		return fmt.Errorf("unsupported source format: %s (supported: 'dotenv', 'ksvc')", convertFrom)
	}

	// Load existing ws.yaml if it exists, or create new config
	var kubaConfig *config.SecretsConfig
	var existingRawContent []byte
	var existingFileExists bool

	if _, err := os.Stat(outPath); err == nil {
		existingFileExists = true
		logger.Debug("Loading existing ws.yaml", "path", outPath)

		// Read raw content to potentially preserve comments
		existingRawContent, err = os.ReadFile(outPath)
		if err != nil {
			return fmt.Errorf("failed to read existing ws.yaml: %w", err)
		}

		// Also load as config struct for manipulation
		kubaConfig, err = config.LoadSecretsConfig(outPath)
		if err != nil {
			return fmt.Errorf("failed to load existing ws.yaml: %w", err)
		}
		logger.Debug("Loaded existing ws.yaml", "environments_count", len(kubaConfig.Environments))
	} else {
		existingFileExists = false
		logger.Debug("No existing ws.yaml found, creating new config")
		kubaConfig = &config.SecretsConfig{
			Environments: make(map[string]config.Environment),
		}
	}

	// Create or update the environment
	env, exists := kubaConfig.Environments[convertEnv]
	if !exists {
		logger.Debug("Creating new environment", "env", convertEnv)
		// Create a new environment
		env = config.Environment{
			Provider: defaultProvider,
			Project:  defaultProject,
			Env:      make(map[string]config.EnvItem),
		}
	} else {
		logger.Debug("Updating existing environment", "env", convertEnv)
		if env.Env == nil {
			env.Env = make(map[string]config.EnvItem)
		}
	}

	// Add or update entries in the environment
	for key, item := range newEnvItems {
		env.Env[key] = item
		logger.Debug("Added or updated environment variable", "key", key)
	}

	// Clean up empty values from the environment before writing
	cleanupEmptyValues(&env)

	// Update the environment in config
	kubaConfig.Environments[convertEnv] = env

	// Write the updated ws.yaml
	logger.Debug("Writing ws.yaml", "path", outPath)
	if err := writeSecretsConfigWithCommentPreservation(outPath, kubaConfig, existingRawContent, existingFileExists); err != nil {
		return fmt.Errorf("failed to write ws.yaml: %w", err)
	}

	fmt.Printf("Successfully converted %d variables from %s to ws.yaml (environment: %s)\n", sourceVarsCount, convertInfile, convertEnv)
	logger.Debug("Conversion completed successfully")
	return nil
}

// parseKsvcFile reads and parses a Knative Service (ksvc) YAML file and converts
// its container environment variables into kuba EnvItems.
// It returns the env items, along with a suggested default provider and project.
// ksvcEnv represents a single environment variable entry in a Knative Service spec.
type ksvcEnv struct {
	Name      string `yaml:"name"`
	Value     string `yaml:"value,omitempty"`
	ValueFrom struct {
		SecretKeyRef struct {
			Name string `yaml:"name"`
			Key  string `yaml:"key"`
		} `yaml:"secretKeyRef"`
	} `yaml:"valueFrom,omitempty"`
}

type ksvcContainer struct {
	Env []ksvcEnv `yaml:"env"`
}

type ksvcSpecTemplateSpec struct {
	Containers []ksvcContainer `yaml:"containers"`
}

type ksvcSpecTemplate struct {
	Spec ksvcSpecTemplateSpec `yaml:"spec"`
}

type ksvcSpec struct {
	Template ksvcSpecTemplate `yaml:"template"`
}

type ksvcMetadata struct {
	Namespace string `yaml:"namespace"`
}

type ksvcRoot struct {
	Metadata ksvcMetadata `yaml:"metadata"`
	Spec     ksvcSpec     `yaml:"spec"`
}

func parseKsvcFile(filePath string) (map[string]config.EnvItem, string, string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to read file: %w", err)
	}

	return parseKsvcYAML(data)
}

// parseKsvcYAML parses the raw YAML of a Knative Service (ksvc) and converts
// its container environment variables into kuba EnvItems.
// It returns the env items, along with a suggested default provider and project.
func parseKsvcYAML(data []byte) (map[string]config.EnvItem, string, string, error) {
	var svc ksvcRoot
	if err := yaml.Unmarshal(data, &svc); err != nil {
		return nil, "", "", fmt.Errorf("failed to unmarshal ksvc yaml: %w", err)
	}

	envItems := make(map[string]config.EnvItem)

	// Iterate all containers and their env vars; last one wins on duplicates
	for _, container := range svc.Spec.Template.Spec.Containers {
		for _, e := range container.Env {
			if e.Name == "" {
				continue
			}

			// Hard-coded value
			if strings.TrimSpace(e.Value) != "" {
				envItems[e.Name] = config.EnvItem{
					Value: e.Value,
				}
				continue
			}

			// Secret reference
			if e.ValueFrom.SecretKeyRef.Name != "" {
				// We treat the Kubernetes secret name as the secret-key identifier.
				// The key (often "latest") typically represents the version and is
				// intentionally not modeled here; providers usually default to latest.
				envItems[e.Name] = config.EnvItem{
					SecretKey: e.ValueFrom.SecretKeyRef.Name,
				}
				continue
			}
		}
	}

	// Suggested provider/project defaults for the created environment.
	// For Cloud Run/Knative on GCP the namespace is typically the project number.
	suggestedProvider := "gcp"
	suggestedProject := strings.TrimSpace(svc.Metadata.Namespace)

	return envItems, suggestedProvider, suggestedProject, nil
}

// loadKsvcFromProvider fetches a Knative-style service (e.g. Cloud Run or
// equivalents) from the cloud provider API and converts it into env items.
// It supports:
// - GCP Cloud Run ("gcp")
// - AWS App Runner ("aws")
// - Azure Container Apps ("azure")
func loadKsvcFromProvider(provider, project, name string) (map[string]config.EnvItem, string, string, error) {
	provider = strings.TrimSpace(strings.ToLower(provider))
	project = strings.TrimSpace(project)
	name = strings.TrimSpace(name)

	if provider == "" {
		return nil, "", "", fmt.Errorf("provider is required to load ksvc from provider")
	}
	if project == "" || name == "" {
		return nil, "", "", fmt.Errorf("both project and name are required to load ksvc from provider")
	}

	switch provider {
	case "gcp":
		return loadKsvcFromGCP(project, name)
	case "aws":
		return loadKsvcFromAWS(project, name)
	case "azure":
		return loadKsvcFromAzure(project, name)
	default:
		return nil, "", "", fmt.Errorf("unsupported provider %q for ksvc import (supported: gcp, aws, azure)", provider)
	}
}

// loadKsvcFromGCP loads a Cloud Run service and extracts env vars.
// project is the GCP project ID or number, name is the Cloud Run service name.
func loadKsvcFromGCP(project, name string) (map[string]config.EnvItem, string, string, error) {
	ctx := context.Background()

	runService, err := run.NewService(ctx, option.WithScopes(run.CloudPlatformScope))
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to create Cloud Run client: %w", err)
	}

	svcClient := run.NewProjectsLocationsServicesService(runService)

	// List services across all regions for this project and find the one with
	// the matching metadata.name. This avoids needing an explicit region flag
	// while still working with the Cloud Run Admin API resource model.
	parent := fmt.Sprintf("projects/%s/locations/-", project)

	var svc *run.Service
	listCall := svcClient.List(parent).Context(ctx)
	for {
		resp, err := listCall.Do()
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to list Cloud Run services for project %q: %w", project, err)
		}
		for _, item := range resp.Items {
			if item != nil && item.Metadata != nil && item.Metadata.Name == name {
				svc = item
				break
			}
		}
		nextToken := ""
		if resp.Metadata != nil {
			nextToken = resp.Metadata.Continue
		}

		if svc != nil || nextToken == "" {
			break
		}
		listCall = listCall.Continue(nextToken)
	}

	if svc == nil {
		return nil, "", "", fmt.Errorf("could not find Cloud Run service %q in project %q", name, project)
	}

	envItems := make(map[string]config.EnvItem)

	if svc.Spec != nil && svc.Spec.Template != nil && svc.Spec.Template.Spec != nil {
		for _, container := range svc.Spec.Template.Spec.Containers {
			if container == nil {
				continue
			}
			for _, e := range container.Env {
				if e == nil || strings.TrimSpace(e.Name) == "" {
					continue
				}

				if strings.TrimSpace(e.Value) != "" {
					envItems[e.Name] = config.EnvItem{
						Value: e.Value,
					}
					continue
				}

				if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil && strings.TrimSpace(e.ValueFrom.SecretKeyRef.Name) != "" {
					envItems[e.Name] = config.EnvItem{
						SecretKey: e.ValueFrom.SecretKeyRef.Name,
					}
					continue
				}
			}
		}
	}

	return envItems, "gcp", project, nil
}

// parseAWSServiceAndRegion splits a name of the form "service.region"
// (e.g. "my-service.us-east-1") into service name and region.
func parseAWSServiceAndRegion(name string) (string, string, error) {
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid AWS service name %q, expected format 'service.region'", name)
	}
	region := parts[len(parts)-1]
	serviceName := strings.Join(parts[:len(parts)-1], ".")
	if strings.TrimSpace(serviceName) == "" || strings.TrimSpace(region) == "" {
		return "", "", fmt.Errorf("invalid AWS service name %q, expected non-empty service and region", name)
	}
	return serviceName, region, nil
}

// loadKsvcFromAWS loads environment variables from an AWS App Runner service.
// project is the AWS account ID (not strictly required for the API call),
// name is "service.region" (for example "my-service.us-east-1").
func loadKsvcFromAWS(project, name string) (map[string]config.EnvItem, string, string, error) {
	serviceName, region, err := parseAWSServiceAndRegion(name)
	if err != nil {
		return nil, "", "", err
	}

	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to load AWS config for region %q: %w", region, err)
	}

	client := apprunner.NewFromConfig(cfg)

	// Find the service ARN by listing services and matching by name.
	listOut, err := client.ListServices(ctx, &apprunner.ListServicesInput{})
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to list App Runner services in region %q: %w", region, err)
	}

	var serviceArn string
	for _, s := range listOut.ServiceSummaryList {
		if s.ServiceName != nil && *s.ServiceName == serviceName && s.ServiceArn != nil {
			serviceArn = *s.ServiceArn
			break
		}
	}

	if serviceArn == "" {
		return nil, "", "", fmt.Errorf("could not find App Runner service %q in region %q", serviceName, region)
	}

	descOut, err := client.DescribeService(ctx, &apprunner.DescribeServiceInput{
		ServiceArn: &serviceArn,
	})
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to describe App Runner service %q: %w", serviceName, err)
	}

	envItems := make(map[string]config.EnvItem)

	if descOut.Service != nil &&
		descOut.Service.SourceConfiguration != nil &&
		descOut.Service.SourceConfiguration.ImageRepository != nil &&
		descOut.Service.SourceConfiguration.ImageRepository.ImageConfiguration != nil {

		imgCfg := descOut.Service.SourceConfiguration.ImageRepository.ImageConfiguration

		// Plain environment variables (map[string]string)
		for name, value := range imgCfg.RuntimeEnvironmentVariables {
			if strings.TrimSpace(name) == "" {
				continue
			}
			if strings.TrimSpace(value) == "" {
				continue
			}
			envItems[name] = config.EnvItem{
				Value: value,
			}
		}

		// Secrets-backed env vars (Secrets Manager) - map[string]string
		for name, arn := range imgCfg.RuntimeEnvironmentSecrets {
			if strings.TrimSpace(name) == "" {
				continue
			}
			if strings.TrimSpace(arn) == "" {
				continue
			}
			parts := strings.Split(arn, ":")
			secretID := arn
			if len(parts) >= 6 {
				// arn:partition:service:region:account-id:resource
				secretID = parts[len(parts)-1]
			}
			envItems[name] = config.EnvItem{
				SecretKey: secretID,
			}
		}
	}

	return envItems, "aws", project, nil
}

// parseAzureAppAndResourceGroup splits a name of the form
// "app.resource-group" into app name and resource group name.
func parseAzureAppAndResourceGroup(name string) (string, string, error) {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid Azure service name %q, expected format 'app.resource-group'", name)
	}
	app := strings.TrimSpace(parts[0])
	rg := strings.TrimSpace(parts[1])
	if app == "" || rg == "" {
		return "", "", fmt.Errorf("invalid Azure service name %q, expected non-empty app and resource-group", name)
	}
	return app, rg, nil
}

// loadKsvcFromAzure loads environment variables from an Azure Container App.
// project is the Azure subscription ID, name is "app.resource-group".
func loadKsvcFromAzure(subscriptionID, name string) (map[string]config.EnvItem, string, string, error) {
	appName, rg, err := parseAzureAppAndResourceGroup(name)
	if err != nil {
		return nil, "", "", err
	}

	ctx := context.Background()

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to create Azure credential: %w", err)
	}

	client, err := armappcontainers.NewContainerAppsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to create Azure Container Apps client: %w", err)
	}

	resp, err := client.Get(ctx, rg, appName, nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to fetch Azure Container App %q in resource group %q: %w", appName, rg, err)
	}

	envItems := make(map[string]config.EnvItem)

	if resp.Properties != nil && resp.Properties.Template != nil {
		for _, c := range resp.Properties.Template.Containers {
			if c == nil || c.Env == nil {
				continue
			}
			for _, ev := range c.Env {
				if ev == nil || ev.Name == nil || strings.TrimSpace(*ev.Name) == "" {
					continue
				}

				// Secrets-backed env vars reference an app secret by name via SecretRef.
				if ev.SecretRef != nil && strings.TrimSpace(*ev.SecretRef) != "" {
					envItems[*ev.Name] = config.EnvItem{
						SecretKey: *ev.SecretRef,
					}
					continue
				}

				if ev.Value != nil && strings.TrimSpace(*ev.Value) != "" {
					envItems[*ev.Name] = config.EnvItem{
						Value: *ev.Value,
					}
					continue
				}
			}
		}
	}

	return envItems, "azure", subscriptionID, nil
}

// parseDotenvFile reads and parses a dotenv file
// It handles:
// - Comments (lines starting with #)
// - Blank lines
// - KEY=VALUE pairs
// - Quoted values (single and double quotes)
// - Multiline values (basic support)
func parseDotenvFile(filePath string) (map[string]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	envVars := make(map[string]string)
	scanner := bufio.NewScanner(file)

	var currentKey string
	var currentValue strings.Builder

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" {
			continue
		}

		// Skip comments
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Check if this line continues a previous multiline value
		if currentKey != "" && (strings.HasPrefix(line, "\"") || strings.HasPrefix(line, "'")) {
			// This might be a continuation, but for simplicity, we'll treat each line independently
			currentKey = ""
			currentValue.Reset()
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			// Try to continue a multiline value
			if currentKey != "" {
				currentValue.WriteString("\n")
				currentValue.WriteString(line)
				continue
			}
			// Skip malformed lines
			continue
		}

		// If we had a previous key being built, save it now
		if currentKey != "" {
			envVars[currentKey] = strings.TrimSpace(currentValue.String())
			currentKey = ""
			currentValue.Reset()
		}

		key := strings.TrimSpace(parts[0])
		valueStr := strings.TrimSpace(parts[1])

		// Skip empty keys
		if key == "" {
			continue
		}

		// Handle quoted values
		valueStr = unquoteValue(valueStr)

		// Check for multiline value (values ending with \)
		if strings.HasSuffix(valueStr, "\\") && !strings.HasSuffix(valueStr, "\\\\") {
			// Start building a multiline value
			currentKey = key
			currentValue.WriteString(strings.TrimSuffix(valueStr, "\\"))
			continue
		}

		envVars[key] = valueStr
	}

	// Handle any remaining multiline value
	if currentKey != "" {
		envVars[currentKey] = strings.TrimSpace(currentValue.String())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return envVars, nil
}

// unquoteValue removes surrounding quotes from a value if present
func unquoteValue(value string) string {
	value = strings.TrimSpace(value)

	// Handle double quotes
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		// Remove quotes and unescape
		unquoted := value[1 : len(value)-1]
		// Basic unescaping for common cases
		unquoted = strings.ReplaceAll(unquoted, "\\n", "\n")
		unquoted = strings.ReplaceAll(unquoted, "\\t", "\t")
		unquoted = strings.ReplaceAll(unquoted, "\\\"", "\"")
		unquoted = strings.ReplaceAll(unquoted, "\\\\", "\\")
		return unquoted
	}

	// Handle single quotes
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1]
	}

	return value
}

// cleanupEmptyValues removes environment variables with empty values from the environment
// This ensures that empty values are not written to the YAML file
func cleanupEmptyValues(env *config.Environment) {
	cleanedEnv := make(map[string]config.EnvItem)
	for key, item := range env.Env {
		// Check if the item has any meaningful content
		hasContent := false

		// Check value
		if item.Value != nil {
			valueStr := fmt.Sprintf("%v", item.Value)
			if strings.TrimSpace(valueStr) != "" {
				hasContent = true
			}
		}

		// Check secret-key
		if item.SecretKey != "" {
			hasContent = true
		}

		// Check secret-path
		if len(item.SecretPath) > 0 {
			hasContent = true
		}

		// Only include items that have some content
		if hasContent {
			cleanedEnv[key] = item
		}
	}
	env.Env = cleanedEnv
}

// writeSecretsConfigWithCommentPreservation writes a SecretsConfig to a YAML file
// It attempts to preserve comments when updating existing files by using yaml.Node
func writeSecretsConfigWithCommentPreservation(filePath string, cfg *config.SecretsConfig, existingRawContent []byte, existingFileExists bool) error {
	schemaComment := "# yaml-language-server: $schema=https://withsecrets.com/ws.schema.json\n---\n"
	// Ensure the directory exists
	dir := filepath.Dir(filePath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// If we have existing content, try to preserve comments using yaml.Node
	if existingFileExists && len(existingRawContent) > 0 {
		// Parse existing YAML into a node tree (preserves comments)
		var existingNode yaml.Node
		if err := yaml.Unmarshal(existingRawContent, &existingNode); err == nil {
			// Try to update only the specific environment section
			// This is a best-effort attempt - some comments may still be lost
			if err := updateEnvironmentInNode(&existingNode, convertEnv, cfg.Environments[convertEnv]); err == nil {
				// Successfully updated the node tree, write it back
				var buf strings.Builder
				encoder := yaml.NewEncoder(&buf)
				encoder.SetIndent(2)
				if err := encoder.Encode(&existingNode); err == nil {
					encoder.Close()
					content := buf.String()

					// Ensure schema comment is present
					if !strings.Contains(content, "yaml-language-server") {
						content = schemaComment + content
					}

					return os.WriteFile(filePath, []byte(content), 0644)
				}
			}
			// If node-based update failed, fall through to struct-based marshaling
		}
	}

	// Fallback: marshal from struct (comments will be lost, but structure is correct)
	var buf strings.Builder
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}
	encoder.Close()

	content := buf.String()
	// Add schema comment at the top if file is new or doesn't have it
	if !strings.Contains(content, schemaComment) {
		content = schemaComment + content
	}

	// Write file
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// updateEnvironmentInNode updates a specific environment section in a yaml.Node tree
// NOTE: This function replaces the entire environment node, which means comments within
// that environment section will be lost. Comments in other environments are preserved.
func updateEnvironmentInNode(rootNode *yaml.Node, envName string, env config.Environment) error {
	// The root node should be a document node
	if rootNode.Kind != yaml.DocumentNode && rootNode.Kind != yaml.MappingNode {
		return fmt.Errorf("unexpected root node kind: %v", rootNode.Kind)
	}

	// Find the mapping node (the actual content)
	var mappingNode *yaml.Node
	if rootNode.Kind == yaml.DocumentNode && len(rootNode.Content) > 0 {
		mappingNode = rootNode.Content[0]
	} else {
		mappingNode = rootNode
	}

	if mappingNode.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping node, got %v", mappingNode.Kind)
	}

	// Find the environment key-value pair
	envNodeIndex := -1
	for i := 0; i < len(mappingNode.Content); i += 2 {
		if i+1 < len(mappingNode.Content) {
			keyNode := mappingNode.Content[i]
			if keyNode.Value == envName {
				envNodeIndex = i + 1
				break
			}
		}
	}

	// Create new environment node from the config
	// WARNING: This replaces the entire node, losing all comments within this environment section
	newEnvNode := &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
	}

	// Add provider
	newEnvNode.Content = append(newEnvNode.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "provider"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: env.Provider},
	)

	// Add project if present and not empty
	if strings.TrimSpace(env.Project) != "" {
		newEnvNode.Content = append(newEnvNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "project"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: env.Project},
		)
	}

	// Add env map
	envMapNode := &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
	}
	for key, item := range env.Env {
		itemNode := &yaml.Node{
			Kind: yaml.MappingNode,
			Tag:  "!!map",
		}
		hasContent := false

		// Only add value if it's non-empty
		if item.Value != nil {
			valueStr := fmt.Sprintf("%v", item.Value)
			if strings.TrimSpace(valueStr) != "" {
				itemNode.Content = append(itemNode.Content,
					&yaml.Node{Kind: yaml.ScalarNode, Value: "value"},
					&yaml.Node{Kind: yaml.ScalarNode, Value: valueStr},
				)
				hasContent = true
			}
		}

		// Add secret-key if present
		if item.SecretKey != "" {
			itemNode.Content = append(itemNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "secret-key"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: item.SecretKey},
			)
			hasContent = true
		}

		// Add secret-path if present
		if len(item.SecretPath) > 0 {
			itemNode.Content = append(itemNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "secret-path"},
				config.PathValueNode(item.SecretPath),
			)
			hasContent = true
		}

		// Add provider if present and different from env-level provider
		if item.Provider != "" && item.Provider != env.Provider {
			itemNode.Content = append(itemNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "provider"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: item.Provider},
			)
			hasContent = true
		}

		// Add project if present and different from env-level project
		if item.Project != "" && item.Project != env.Project {
			itemNode.Content = append(itemNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "project"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: item.Project},
			)
			hasContent = true
		}

		// Only add the env item if it has some content
		if hasContent {
			envMapNode.Content = append(envMapNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: key},
				itemNode,
			)
		}
	}
	newEnvNode.Content = append(newEnvNode.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "env"},
		envMapNode,
	)

	// Update or add the environment
	if envNodeIndex >= 0 {
		// Update existing - this replaces the node, losing comments
		mappingNode.Content[envNodeIndex] = newEnvNode
	} else {
		// Add new environment
		mappingNode.Content = append(mappingNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: envName},
			newEnvNode,
		)
	}

	return nil
}
