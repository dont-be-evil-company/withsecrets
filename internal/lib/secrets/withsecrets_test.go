package secrets

import (
	"context"
	"strings"
	"testing"
)

func TestWithSecretsProviderDoesNotFetch(t *testing.T) {
	t.Setenv("WITHSECRETS_SERVER", "https://vault.example.com")
	mgr, err := NewSecretManagerFactory().CreateSecretManager(context.Background(), "withsecrets", "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = mgr.GetSecret("project", "database-url")
	if err == nil || !strings.Contains(err.Error(), "unattended") {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), "vault.example.com") {
		t.Fatal("provider contacted or echoed the server")
	}
}

func TestCloudProvidersIgnoreVaultEnv(t *testing.T) {
	t.Setenv("WITHSECRETS_SERVER", "https://vault.example.com")
	_, err := NewSecretManagerFactory().CreateSecretManager(context.Background(), "not-a-provider", "", "")
	if err == nil || strings.Contains(err.Error(), "vault.example.com") {
		t.Fatalf("got %v", err)
	}
}
