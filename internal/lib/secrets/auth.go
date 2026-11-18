package secrets

import (
	"context"
	"fmt"
	"os"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	sdk "github.com/bitwarden/sdk-go/v2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// AuthorizationTestResult contains the result of an authorization test
type AuthorizationTestResult struct {
	Provider        string
	ProjectID       string
	Authenticated   bool
	HasPermissions  bool
	CredentialsInfo string
	ErrorMessage    string
	ExampleSecret   string
}

// TestGCPAuthorization tests GCP credentials and permissions
func TestGCPAuthorization(ctx context.Context, projectID string) (*AuthorizationTestResult, error) {
	result := &AuthorizationTestResult{
		Provider:  "gcp",
		ProjectID: projectID,
	}

	// Step 1: Check if Application Default Credentials exist
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		result.Authenticated = false
		result.ErrorMessage = fmt.Sprintf("Not logged in or invalid credentials: %v", err)
		result.CredentialsInfo = "No Application Default Credentials found. Run 'gcloud auth application-default login' or set GOOGLE_APPLICATION_CREDENTIALS."
		return result, nil
	}

	result.Authenticated = true
	if creds.ProjectID != "" {
		result.CredentialsInfo = fmt.Sprintf("Found credentials for project: %s", creds.ProjectID)
	} else {
		result.CredentialsInfo = "Found credentials (project ID not specified in credentials)"
	}

	// Step 2: Create Secret Manager client directly for testing
	credentialsFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	var opts []option.ClientOption
	if credentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(credentialsFile))
	}

	secretmanagerClient, err := secretmanager.NewClient(ctx, opts...)
	if err != nil {
		result.HasPermissions = false
		result.ErrorMessage = fmt.Sprintf("Failed to create Secret Manager client: %v", err)
		return result, nil
	}
	defer secretmanagerClient.Close()

	// Step 3: Try listing secrets to verify access
	// Use the project ID from credentials if not provided
	testProjectID := projectID
	if testProjectID == "" {
		testProjectID = creds.ProjectID
	}
	if testProjectID == "" {
		result.HasPermissions = false
		result.ErrorMessage = "Project ID is required but not found in credentials or configuration"
		return result, nil
	}

	req := &secretmanagerpb.ListSecretsRequest{
		Parent: fmt.Sprintf("projects/%s", testProjectID),
	}

	it := secretmanagerClient.ListSecrets(ctx, req)
	secret, err := it.Next()
	if err == iterator.Done {
		// No secrets found, but we have permissions (empty list is valid)
		result.HasPermissions = true
		result.CredentialsInfo += " (No secrets found in project, but access is working)"
		return result, nil
	}
	if err != nil {
		result.HasPermissions = false
		result.ErrorMessage = fmt.Sprintf("Authenticated, but could not list secrets (possibly lack permissions): %v", err)
		return result, nil
	}

	// Success - we found at least one secret
	result.HasPermissions = true
	if secret != nil {
		// Extract just the secret name from the full path
		secretName := extractSecretNameFromPath(secret.Name)
		result.ExampleSecret = secretName
		result.CredentialsInfo += fmt.Sprintf(" - Successfully authenticated! Example secret found: %s", secretName)
	} else {
		result.CredentialsInfo += " - Successfully authenticated and can list secrets!"
	}

	return result, nil
}

// TestAWSAuthorization tests AWS credentials and permissions
func TestAWSAuthorization(ctx context.Context, projectID string) (*AuthorizationTestResult, error) {
	result := &AuthorizationTestResult{
		Provider:  "aws",
		ProjectID: projectID,
	}

	// Step 1: Try to create AWS client (this will check credentials)
	region := os.Getenv("AWS_REGION")
	profile := os.Getenv("AWS_PROFILE")
	client, err := NewAWSSecretsManager(ctx, region, profile)
	if err != nil {
		result.Authenticated = false
		result.ErrorMessage = fmt.Sprintf("Failed to load AWS credentials: %v", err)
		result.CredentialsInfo = "No valid AWS credentials found. Set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY, or configure AWS CLI with 'aws configure'."
		return result, nil
	}

	result.Authenticated = true
	if region != "" {
		result.CredentialsInfo = fmt.Sprintf("Found AWS credentials for region: %s", region)
	} else {
		result.CredentialsInfo = "Found AWS credentials (using default region)"
	}

	// Step 2: Try listing Secrets Manager secrets and/or Parameter Store parameters
	secretNames, secretsErr := client.ListSecrets()
	paramNames, paramsErr := client.ListParameters()

	if secretsErr != nil && paramsErr != nil {
		result.HasPermissions = false
		result.ErrorMessage = fmt.Sprintf("Authenticated, but could not list Secrets Manager secrets (%v) or Parameter Store parameters (%v)", secretsErr, paramsErr)
		return result, nil
	}

	result.HasPermissions = true
	var details []string
	if secretsErr == nil {
		if len(secretNames) > 0 {
			result.ExampleSecret = secretNames[0]
			details = append(details, fmt.Sprintf("Secrets Manager OK (example: %s)", secretNames[0]))
		} else {
			details = append(details, "Secrets Manager OK (no secrets found)")
		}
	} else {
		details = append(details, fmt.Sprintf("Secrets Manager list failed: %v", secretsErr))
	}
	if paramsErr == nil {
		if len(paramNames) > 0 {
			if result.ExampleSecret == "" {
				result.ExampleSecret = paramNames[0]
			}
			details = append(details, fmt.Sprintf("Parameter Store OK (example: %s)", paramNames[0]))
		} else {
			details = append(details, "Parameter Store OK (no parameters found)")
		}
	} else {
		details = append(details, fmt.Sprintf("Parameter Store list failed: %v", paramsErr))
	}
	result.CredentialsInfo += " - Successfully authenticated! " + strings.Join(details, "; ")

	return result, nil
}

// TestAzureAuthorization tests Azure credentials and permissions
func TestAzureAuthorization(ctx context.Context, projectID string) (*AuthorizationTestResult, error) {
	result := &AuthorizationTestResult{
		Provider:  "azure",
		ProjectID: projectID,
	}

	// Step 1: Check for required Azure Key Vault URL
	vaultURL := os.Getenv("AZURE_KEY_VAULT_URL")
	if vaultURL == "" {
		result.Authenticated = false
		result.ErrorMessage = "AZURE_KEY_VAULT_URL environment variable is required for Azure Key Vault"
		result.CredentialsInfo = "Set AZURE_KEY_VAULT_URL environment variable to your Key Vault URL."
		return result, nil
	}

	// Step 2: Try to create Azure client (this will check credentials)
	tenantID := os.Getenv("AZURE_TENANT_ID")
	clientID := os.Getenv("AZURE_CLIENT_ID")
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET")
	client, err := NewAzureKeyVaultManager(ctx, vaultURL, tenantID, clientID, clientSecret)
	if err != nil {
		result.Authenticated = false
		result.ErrorMessage = fmt.Sprintf("Failed to create Azure Key Vault client: %v", err)
		result.CredentialsInfo = "No valid Azure credentials found. Set AZURE_TENANT_ID, AZURE_CLIENT_ID, and AZURE_CLIENT_SECRET, or use Azure CLI authentication."
		return result, nil
	}

	result.Authenticated = true
	result.CredentialsInfo = fmt.Sprintf("Found Azure credentials for Key Vault: %s", vaultURL)

	// Step 3: Try listing secrets to verify access
	secretNames, err := client.ListSecrets()
	if err != nil {
		result.HasPermissions = false
		result.ErrorMessage = fmt.Sprintf("Authenticated, but could not list secrets (possibly lack permissions): %v", err)
		return result, nil
	}

	// Success
	result.HasPermissions = true
	if len(secretNames) > 0 {
		result.ExampleSecret = secretNames[0]
		result.CredentialsInfo += fmt.Sprintf(" - Successfully authenticated! Example secret found: %s", secretNames[0])
	} else {
		result.CredentialsInfo += " - Successfully authenticated! (No secrets found, but access is working)"
	}

	return result, nil
}

// TestOpenBaoAuthorization tests OpenBao connection and permissions
func TestOpenBaoAuthorization(ctx context.Context, projectID string) (*AuthorizationTestResult, error) {
	result := &AuthorizationTestResult{
		Provider:  "openbao",
		ProjectID: projectID,
	}

	// OPENBAO_* overrides the client's BAO_* / VAULT_* values. An empty address leaves BAO_ADDR in place.
	address := os.Getenv("OPENBAO_ADDR")
	token := os.Getenv("OPENBAO_TOKEN")
	namespace := os.Getenv("OPENBAO_NAMESPACE")
	client, err := NewOpenBaoManager(ctx, address, token, namespace)
	if err != nil {
		result.Authenticated = false
		result.ErrorMessage = fmt.Sprintf("Failed to create OpenBao client: %v", err)
		result.CredentialsInfo = "Failed to connect to OpenBao. Check OPENBAO_ADDR or BAO_ADDR, and a token from OPENBAO_TOKEN, BAO_TOKEN, BAO_TOKEN_FILE, or ~/.bao-token."
		return result, nil
	}

	result.Authenticated = true
	result.CredentialsInfo = fmt.Sprintf("Connected to OpenBao at: %s", client.client.Address())

	// Step 3: Try listing secrets to verify access
	// Try listing at root or a common path
	secretNames, err := client.ListSecrets("secret")
	if err != nil {
		// Try listing at root
		secretNames, err = client.ListSecrets("")
		if err != nil {
			result.HasPermissions = false
			result.ErrorMessage = fmt.Sprintf("Connected, but could not list secrets (possibly lack permissions or invalid path): %v", err)
			return result, nil
		}
	}

	// Success
	result.HasPermissions = true
	if len(secretNames) > 0 {
		result.ExampleSecret = secretNames[0]
		result.CredentialsInfo += fmt.Sprintf(" - Successfully connected! Example secret found: %s", secretNames[0])
	} else {
		result.CredentialsInfo += " - Successfully connected! (No secrets found at tested path, but access is working)"
	}

	return result, nil
}

// TestLocalAuthorization tests local provider (always succeeds, no auth needed)
func TestLocalAuthorization(ctx context.Context, projectID string) (*AuthorizationTestResult, error) {
	result := &AuthorizationTestResult{
		Provider:        "local",
		ProjectID:       projectID,
		Authenticated:   true,
		HasPermissions:  true,
		CredentialsInfo: "Local provider uses environment variables - no authentication required.",
	}
	return result, nil
}

// TestAuthorization tests authorization for a specific provider
func (f *SecretManagerFactory) TestAuthorization(ctx context.Context, provider string, projectID string) (*AuthorizationTestResult, error) {
	switch provider {
	case "gcp":
		return TestGCPAuthorization(ctx, projectID)
	case "aws":
		return TestAWSAuthorization(ctx, projectID)
	case "azure":
		return TestAzureAuthorization(ctx, projectID)
	case "openbao":
		return TestOpenBaoAuthorization(ctx, projectID)
	case "local":
		return TestLocalAuthorization(ctx, projectID)
	case "bitwarden":
		return TestBitwardenAuthorization(ctx, projectID)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

// TestBitwardenAuthorization tests Bitwarden connection and permissions
func TestBitwardenAuthorization(ctx context.Context, projectID string) (*AuthorizationTestResult, error) {
	result := &AuthorizationTestResult{
		Provider:  "bitwarden",
		ProjectID: projectID,
	}

	// Determine organization ID (from projectID or env)
	organizationID := projectID
	if organizationID == "" {
		organizationID = os.Getenv("BITWARDEN_ORGANIZATION_ID")
	}
	if organizationID == "" {
		result.Authenticated = false
		result.ErrorMessage = "Bitwarden organization ID is required. Set BITWARDEN_ORGANIZATION_ID or configure 'project' for the environment."
		result.CredentialsInfo = "Missing organization ID."
		return result, nil
	}

	accessToken := os.Getenv("BITWARDEN_ACCESS_TOKEN")
	if accessToken == "" {
		accessToken = os.Getenv("ACCESS_TOKEN")
	}
	if accessToken == "" {
		result.Authenticated = false
		result.ErrorMessage = "Bitwarden access token is required. Set BITWARDEN_ACCESS_TOKEN or ACCESS_TOKEN."
		result.CredentialsInfo = "Missing access token."
		return result, nil
	}

	var apiURLPtr *string
	if apiURL := os.Getenv("BITWARDEN_API_URL"); apiURL != "" {
		apiURLCopy := apiURL
		apiURLPtr = &apiURLCopy
	}

	var identityURLPtr *string
	if identityURL := os.Getenv("BITWARDEN_IDENTITY_URL"); identityURL != "" {
		identityURLCopy := identityURL
		identityURLPtr = &identityURLCopy
	}

	client, err := sdk.NewBitwardenClient(apiURLPtr, identityURLPtr)
	if err != nil {
		result.Authenticated = false
		result.ErrorMessage = fmt.Sprintf("Failed to create Bitwarden client: %v", err)
		result.CredentialsInfo = "Unable to initialize Bitwarden SDK client."
		return result, nil
	}
	defer client.Close()

	// Try logging in with the access token
	if err := client.AccessTokenLogin(accessToken, nil); err != nil {
		result.Authenticated = false
		result.ErrorMessage = fmt.Sprintf("Failed to authenticate with Bitwarden: %v", err)
		result.CredentialsInfo = "Access token login failed."
		return result, nil
	}

	result.Authenticated = true
	result.CredentialsInfo = "Successfully authenticated with Bitwarden."

	// Try listing secrets to verify access
	secrets, err := client.Secrets().List(organizationID)
	if err != nil {
		result.HasPermissions = false
		result.ErrorMessage = fmt.Sprintf("Authenticated, but could not list Bitwarden secrets: %v", err)
		return result, nil
	}

	result.HasPermissions = true
	if secrets != nil && len(secrets.Data) > 0 {
		// Use the first secret's key as an example
		result.ExampleSecret = secrets.Data[0].Key
		result.CredentialsInfo += fmt.Sprintf(" Example secret found: %s", secrets.Data[0].Key)
	} else {
		result.CredentialsInfo += " Successfully listed Bitwarden secrets (no secrets found)."
	}

	return result, nil
}
