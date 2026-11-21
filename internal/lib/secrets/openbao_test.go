package secrets

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func isolateOpenBaoEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, key := range []string{
		"OPENBAO_ADDR",
		"OPENBAO_TOKEN",
		"OPENBAO_NAMESPACE",
		"BAO_ADDR",
		"BAO_TOKEN",
		"BAO_TOKEN_FILE",
		"BAO_NAMESPACE",
		"BAO_SKIP_VERIFY",
		"BAO_CACERT",
		"BAO_CACERT_BYTES",
		"BAO_CAPATH",
		"BAO_CLIENT_CERT",
		"BAO_CLIENT_KEY",
		"BAO_CLIENT_TIMEOUT",
		"VAULT_ADDR",
		"VAULT_TOKEN",
		"VAULT_TOKEN_FILE",
		"VAULT_NAMESPACE",
		"VAULT_SKIP_VERIFY",
		"VAULT_CACERT",
		"VAULT_CAPATH",
		"VAULT_CLIENT_CERT",
		"VAULT_CLIENT_KEY",
		"VAULT_CLIENT_TIMEOUT",
	} {
		t.Setenv(key, "")
	}
	return home
}

func writeTokenFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
}

func TestNewOpenBaoManager(t *testing.T) {
	ctx := context.Background()

	// Test with minimal configuration
	manager, err := NewOpenBaoManager(ctx, "http://localhost:8200", "", "")
	if err != nil {
		t.Fatalf("Failed to create OpenBao manager: %v", err)
	}

	if manager == nil {
		t.Error("Expected manager to be created")
	}

	if manager.client == nil {
		t.Error("Expected client to be initialized")
	}

	// Clean up
	if err := manager.Close(); err != nil {
		t.Errorf("Failed to close manager: %v", err)
	}
}

func TestNewOpenBaoManager_WithToken(t *testing.T) {
	ctx := context.Background()

	// Test with token
	manager, err := NewOpenBaoManager(ctx, "http://localhost:8200", "test-token", "")
	if err != nil {
		t.Fatalf("Failed to create OpenBao manager: %v", err)
	}

	if manager == nil {
		t.Error("Expected manager to be created")
	}

	if manager.client == nil {
		t.Error("Expected client to be initialized")
	}

	// Clean up
	if err := manager.Close(); err != nil {
		t.Errorf("Failed to close manager: %v", err)
	}
}

func TestNewOpenBaoManager_WithNamespace(t *testing.T) {
	ctx := context.Background()

	// Test with namespace
	manager, err := NewOpenBaoManager(ctx, "http://localhost:8200", "test-token", "test-namespace")
	if err != nil {
		t.Fatalf("Failed to create OpenBao manager: %v", err)
	}

	if manager == nil {
		t.Error("Expected manager to be created")
	}

	if manager.client == nil {
		t.Error("Expected client to be initialized")
	}

	// Clean up
	if err := manager.Close(); err != nil {
		t.Errorf("Failed to close manager: %v", err)
	}
}

func TestOpenBaoManager_GetSecret(t *testing.T) {
	ctx := context.Background()

	// This test requires an actual OpenBao server running
	// In a real test environment, you would mock the client or use a test server
	t.Skip("Skipping test that requires OpenBao server")

	manager, err := NewOpenBaoManager(ctx, "http://localhost:8200", "test-token", "")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// Test getting a secret
	secret, err := manager.GetSecret("", "secret/test")
	if err != nil {
		t.Errorf("Failed to get secret: %v", err)
		return
	}

	if secret == "" {
		t.Error("Expected secret to have a value")
	}
}

func TestOpenBaoManager_GetSecrets(t *testing.T) {
	ctx := context.Background()

	// This test requires an actual OpenBao server running
	// In a real test environment, you would mock the client or use a test server
	t.Skip("Skipping test that requires OpenBao server")

	manager, err := NewOpenBaoManager(ctx, "http://localhost:8200", "test-token", "")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// Test getting multiple secrets
	secretIDs := []string{"secret/test1", "secret/test2"}
	secrets, err := manager.GetSecrets("", secretIDs)
	if err != nil {
		t.Errorf("Failed to get secrets: %v", err)
		return
	}

	if len(secrets) != len(secretIDs) {
		t.Errorf("Expected %d secrets, got %d", len(secretIDs), len(secrets))
	}

	for _, secretID := range secretIDs {
		if _, exists := secrets[secretID]; !exists {
			t.Errorf("Expected secret '%s' to be in results", secretID)
		}
	}
}

func TestOpenBaoManager_ListSecrets(t *testing.T) {
	ctx := context.Background()

	// This test requires an actual OpenBao server running
	// In a real test environment, you would mock the client or use a test server
	t.Skip("Skipping test that requires OpenBao server")

	manager, err := NewOpenBaoManager(ctx, "http://localhost:8200", "test-token", "")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// Test listing secrets
	secrets, err := manager.ListSecrets("secret/")
	if err != nil {
		t.Errorf("Failed to list secrets: %v", err)
		return
	}

	// Should return a list (even if empty)
	if secrets == nil {
		t.Error("Expected secrets list to not be nil")
	}
}

func TestOpenBaoManager_CreateSecret(t *testing.T) {
	ctx := context.Background()

	// This test requires an actual OpenBao server running
	// In a real test environment, you would mock the client or use a test server
	t.Skip("Skipping test that requires OpenBao server")

	manager, err := NewOpenBaoManager(ctx, "http://localhost:8200", "test-token", "")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// Test creating a secret
	data := map[string]interface{}{
		"password": "test-password",
		"username": "test-user",
	}

	err = manager.CreateSecret("secret/test-create", data)
	if err != nil {
		t.Errorf("Failed to create secret: %v", err)
	}
}

func TestOpenBaoManager_UpdateSecret(t *testing.T) {
	ctx := context.Background()

	// This test requires an actual OpenBao server running
	// In a real test environment, you would mock the client or use a test server
	t.Skip("Skipping test that requires OpenBao server")

	manager, err := NewOpenBaoManager(ctx, "http://localhost:8200", "test-token", "")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// Test updating a secret
	data := map[string]interface{}{
		"password": "updated-password",
		"username": "updated-user",
	}

	err = manager.UpdateSecret("secret/test-update", data)
	if err != nil {
		t.Errorf("Failed to update secret: %v", err)
	}
}

func TestOpenBaoManager_DeleteSecret(t *testing.T) {
	ctx := context.Background()

	// This test requires an actual OpenBao server running
	// In a real test environment, you would mock the client or use a test server
	t.Skip("Skipping test that requires OpenBao server")

	manager, err := NewOpenBaoManager(ctx, "http://localhost:8200", "test-token", "")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// Test deleting a secret
	err = manager.DeleteSecret("secret/test-delete")
	if err != nil {
		t.Errorf("Failed to delete secret: %v", err)
	}
}

func TestKV2DataPathInsertsDataSegment(t *testing.T) {
	got := kv2DataPath("secretv2/withsecrets/foo/bar", "secretv2/")
	if got != "secretv2/data/withsecrets/foo/bar" {
		t.Fatalf("kv2DataPath() = %q", got)
	}

	got = kv2DataPath("kv/data/shared/key", "kv")
	if got != "kv/data/shared/key" {
		t.Fatalf("existing data segment was rewritten to %q", got)
	}

	alt, ok := insertKV2DataSegment("secretv2/withsecrets/foo/bar")
	if !ok || alt != "secretv2/data/withsecrets/foo/bar" {
		t.Fatalf("insertKV2DataSegment() = %q, %v", alt, ok)
	}
	if _, ok := insertKV2DataSegment("kv/data/shared/key"); ok {
		t.Fatal("insertKV2DataSegment rewrote a path that already has /data/")
	}
}

func TestSelectSecretValueKV2AndField(t *testing.T) {
	kv2 := map[string]interface{}{
		"data": map[string]interface{}{
			"MY_TOKEN": "tok",
			"OTHER":    "other",
		},
		"metadata": map[string]interface{}{"version": 1},
	}

	got, err := selectSecretValue(kv2, "MY_TOKEN", "kv/data/shared/key")
	if err != nil {
		t.Fatalf("field select: %v", err)
	}
	if got != "tok" {
		t.Fatalf("field select = %q, want tok", got)
	}

	if _, err := selectSecretValue(kv2, "MISSING", "kv/data/shared/key"); err == nil {
		t.Fatal("expected missing field error")
	}

	nonString := map[string]interface{}{
		"data": map[string]interface{}{
			"COUNT": map[string]interface{}{"n": 1},
		},
		"metadata": map[string]interface{}{"version": 1},
	}
	if _, err := selectSecretValue(nonString, "COUNT", "kv/data/shared/key"); err == nil {
		t.Fatal("expected non-string field error")
	}

	got, err = selectSecretValue(kv2, "", "kv/data/shared/key")
	if err != nil {
		t.Fatalf("kv2 without field: %v", err)
	}
	if got != "tok" && got != "other" {
		t.Fatalf("kv2 without field = %q, want one of the inner strings", got)
	}

	kv1 := map[string]interface{}{"password": "secret"}
	got, err = selectSecretValue(kv1, "", "secret/database-url")
	if err != nil {
		t.Fatalf("kv1: %v", err)
	}
	if got != "secret" {
		t.Fatalf("kv1 = %q, want secret", got)
	}

	// A map under "data" without metadata is KV v1 user data, not a KV v2 envelope.
	notV2 := map[string]interface{}{
		"data":  map[string]interface{}{"inner": "x"},
		"other": "y",
	}
	got, err = selectSecretValue(notV2, "", "secret/plain")
	if err != nil {
		t.Fatalf("non-kv2 map: %v", err)
	}
	if got != "y" {
		t.Fatalf("non-kv2 map = %q, want y", got)
	}
}

func TestOpenBaoTokenPrecedence(t *testing.T) {
	ctx := context.Background()
	home := isolateOpenBaoEnv(t)
	tokenFile := filepath.Join(home, "from-file")
	writeTokenFile(t, tokenFile, "from-file\n")
	writeTokenFile(t, filepath.Join(home, ".vault-token"), "from-home\n")
	t.Setenv("BAO_TOKEN_FILE", tokenFile)
	t.Setenv("BAO_TOKEN", "from-bao")
	t.Setenv("VAULT_TOKEN", "from-vault")
	t.Setenv("OPENBAO_TOKEN", "from-openbao")
	t.Setenv("OPENBAO_ADDR", "http://127.0.0.1:8200")

	manager := openBaoFromFactory(t, ctx)
	if got := manager.client.Token(); got != "from-openbao" {
		t.Fatalf("token = %q, want OPENBAO_TOKEN", got)
	}

	t.Setenv("OPENBAO_TOKEN", "")
	manager = openBaoFromFactory(t, ctx)
	if got := manager.client.Token(); got != "from-bao" {
		t.Fatalf("token = %q, want BAO_TOKEN", got)
	}

	t.Setenv("BAO_TOKEN", "")
	t.Setenv("VAULT_TOKEN", "")
	manager = openBaoFromFactory(t, ctx)
	if got := manager.client.Token(); got != "from-file" {
		t.Fatalf("token = %q, want trimmed BAO_TOKEN_FILE", got)
	}

	t.Setenv("BAO_TOKEN_FILE", "")
	manager = openBaoFromFactory(t, ctx)
	if got := manager.client.Token(); got != "from-home" {
		t.Fatalf("token = %q, want ~/.vault-token", got)
	}
}

func TestOpenBaoMissingTokenFileStillCreatesClient(t *testing.T) {
	ctx := context.Background()
	home := isolateOpenBaoEnv(t)
	t.Setenv("BAO_TOKEN_FILE", filepath.Join(home, "missing-token"))
	t.Setenv("OPENBAO_ADDR", "http://127.0.0.1:8200")

	manager := openBaoFromFactory(t, ctx)
	if got := manager.client.Token(); got != "" {
		t.Fatalf("token = %q, want empty when the token file is missing", got)
	}
}

func TestOpenBaoKeepsConfiguredHTTPClient(t *testing.T) {
	ctx := context.Background()
	isolateOpenBaoEnv(t)
	t.Setenv("BAO_SKIP_VERIFY", "true")
	t.Setenv("BAO_ADDR", "https://bao.example:8200")

	manager, err := NewOpenBaoManager(ctx, "", "", "")
	if err != nil {
		t.Fatalf("NewOpenBaoManager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	if got := manager.client.Address(); got != "https://bao.example:8200" {
		t.Fatalf("address = %q, want BAO_ADDR", got)
	}
	if got := manager.client.ClientTimeout(); got != 30*time.Second {
		t.Fatalf("client timeout = %s, want 30s", got)
	}

	cfg := manager.client.CloneConfig()
	if cfg.HttpClient == nil || cfg.HttpClient.Timeout != 30*time.Second {
		t.Fatalf("http client timeout = %v, want 30s", cfg.HttpClient)
	}
	transport, ok := cfg.HttpClient.Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("BAO_SKIP_VERIFY was dropped; the HTTP client was replaced")
	}
	if cfg.HttpClient.CheckRedirect == nil {
		t.Fatal("redirect policy was dropped; the HTTP client was replaced")
	}

	t.Setenv("OPENBAO_ADDR", "https://override.example:8200")
	overridden := openBaoFromFactory(t, ctx)
	if got := overridden.client.Address(); got != "https://override.example:8200" {
		t.Fatalf("address = %q, want OPENBAO_ADDR", got)
	}
}

func openBaoFromFactory(t *testing.T, ctx context.Context) *OpenBaoManager {
	t.Helper()
	manager, err := NewSecretManagerFactory().CreateSecretManager(ctx, "openbao", "", "")
	if err != nil {
		t.Fatalf("CreateSecretManager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	openBao, ok := manager.(*OpenBaoManager)
	if !ok {
		t.Fatalf("manager type = %T, want *OpenBaoManager", manager)
	}
	return openBao
}
