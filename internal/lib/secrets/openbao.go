package secrets

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	vault "github.com/openbao/openbao/api/v2"
)

// OpenBaoManager handles OpenBao operations
type OpenBaoManager struct {
	client *vault.Client
	ctx    context.Context
	mounts map[string]kvMount
}

// kvMount is a KV secrets engine mount discovered from sys/internal/ui/mounts.
type kvMount struct {
	path    string
	version string
}

// NewOpenBaoManager creates a new OpenBao client
func NewOpenBaoManager(ctx context.Context, address string, token string, namespace string) (*OpenBaoManager, error) {
	// Create the client configuration
	config := vault.DefaultConfig()

	// Set the OpenBao server address
	if address != "" {
		config.Address = address
	}

	// Configure HTTP client with reasonable timeouts
	config.HttpClient = &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create the client
	client, err := vault.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	// Set the token if provided
	if token != "" {
		client.SetToken(token)
	}

	// Set the namespace if provided
	if namespace != "" {
		client.SetNamespace(namespace)
	}

	return &OpenBaoManager{
		client: client,
		ctx:    ctx,
		mounts: map[string]kvMount{},
	}, nil
}

// GetSecret retrieves a secret from OpenBao.
// projectID is a path prefix. KV v2 responses are unwrapped before a value is chosen.
func (o *OpenBaoManager) GetSecret(projectID, secretID string) (string, error) {
	data, secretPath, err := o.readSecretData(projectID, secretID)
	if err != nil {
		return "", err
	}
	return selectSecretValue(data, "", secretPath)
}

// GetSecretField retrieves one key from an OpenBao secret, like `bao kv get -field`.
func (o *OpenBaoManager) GetSecretField(projectID, secretID, field string) (string, error) {
	data, secretPath, err := o.readSecretData(projectID, secretID)
	if err != nil {
		return "", err
	}
	return selectSecretValue(data, field, secretPath)
}

func (o *OpenBaoManager) readSecretData(projectID, secretID string) (map[string]interface{}, string, error) {
	secretPath := secretID
	if projectID != "" {
		secretPath = fmt.Sprintf("%s/%s", projectID, secretID)
	}

	readPath := o.kvDataPath(secretPath)
	secret, err := o.client.Logical().Read(readPath)
	if err != nil {
		return nil, readPath, fmt.Errorf("failed to read secret '%s': %w", readPath, err)
	}
	if isVersionedKVPathError(secret) {
		if alt, ok := insertKV2DataSegment(secretPath); ok && alt != readPath {
			readPath = alt
			secret, err = o.client.Logical().Read(readPath)
			if err != nil {
				return nil, readPath, fmt.Errorf("failed to read secret '%s': %w", readPath, err)
			}
		}
	}
	if secret == nil || isVersionedKVPathError(secret) {
		return nil, readPath, fmt.Errorf("secret '%s' not found", secretPath)
	}
	return secret.Data, readPath, nil
}

// kvDataPath inserts the KV v2 /data/ segment after the mount when the joined
// path does not already include it. KV v1 paths and explicit /data/ paths are unchanged.
func (o *OpenBaoManager) kvDataPath(fullPath string) string {
	mount, ok := o.lookupKVMount(fullPath)
	if !ok || mount.version != "2" {
		return fullPath
	}
	return kv2DataPath(fullPath, mount.path)
}

func (o *OpenBaoManager) lookupKVMount(fullPath string) (kvMount, bool) {
	if info, ok := cachedKVMount(o.mounts, fullPath); ok {
		return info, info.version != ""
	}

	secret, err := o.client.Logical().Read("sys/internal/ui/mounts/" + strings.Trim(fullPath, "/"))
	if err != nil || secret == nil || secret.Data == nil {
		return kvMount{}, false
	}
	mountType, _ := secret.Data["type"].(string)
	mountPath, _ := secret.Data["path"].(string)
	version := ""
	if opts, ok := secret.Data["options"].(map[string]interface{}); ok {
		version, _ = opts["version"].(string)
	}
	if mountType != "kv" {
		version = ""
	}
	info := kvMount{path: mountPath, version: version}
	if mountPath != "" {
		o.mounts[mountPath] = info
	}
	if mountType != "kv" || version == "" {
		return kvMount{}, false
	}
	return info, true
}

func cachedKVMount(mounts map[string]kvMount, fullPath string) (kvMount, bool) {
	fullPath = strings.Trim(fullPath, "/")
	var best kvMount
	found := false
	for path, info := range mounts {
		prefix := strings.Trim(path, "/")
		if prefix == "" {
			continue
		}
		if fullPath == prefix || strings.HasPrefix(fullPath, prefix+"/") {
			if !found || len(prefix) > len(strings.Trim(best.path, "/")) {
				best = info
				found = true
			}
		}
	}
	return best, found
}

// kv2DataPath returns the KV v2 read path for a logical secret path.
// fullPath secretv2/withsecrets/foo/bar and mount secretv2 become secretv2/data/withsecrets/foo/bar.
// A path that already contains /data/ after the mount is returned unchanged.
func kv2DataPath(fullPath, mountPath string) string {
	fullPath = strings.Trim(fullPath, "/")
	mountPath = strings.Trim(mountPath, "/")
	if fullPath == "" || mountPath == "" {
		return fullPath
	}
	if fullPath != mountPath && !strings.HasPrefix(fullPath, mountPath+"/") {
		return fullPath
	}
	rel := strings.TrimPrefix(fullPath, mountPath)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || rel == "data" || strings.HasPrefix(rel, "data/") {
		return fullPath
	}
	return mountPath + "/data/" + rel
}

// insertKV2DataSegment inserts /data/ after the first path segment.
// Used when mount discovery is unavailable and OpenBao reports a versioned KV path.
func insertKV2DataSegment(fullPath string) (string, bool) {
	fullPath = strings.Trim(fullPath, "/")
	parts := strings.Split(fullPath, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "data" {
		return fullPath, false
	}
	rewritten := append([]string{parts[0], "data"}, parts[1:]...)
	return strings.Join(rewritten, "/"), true
}

func isVersionedKVPathError(secret *vault.Secret) bool {
	if secret == nil {
		return false
	}
	for _, warning := range secret.Warnings {
		if strings.Contains(warning, "Invalid path for a versioned K/V") {
			return true
		}
	}
	return false
}

// unwrapKV2Data returns the inner KV v2 data map when the response has both
// data and metadata. Other shapes, including KV v1, are returned unchanged.
func unwrapKV2Data(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}
	inner, ok := data["data"].(map[string]interface{})
	if !ok {
		return data
	}
	if _, hasMeta := data["metadata"]; !hasMeta {
		return data
	}
	return inner
}

func selectSecretValue(data map[string]interface{}, field, secretPath string) (string, error) {
	values := unwrapKV2Data(data)
	if len(values) == 0 {
		return "", fmt.Errorf("secret '%s' has no data", secretPath)
	}

	if field != "" {
		value, ok := values[field]
		if !ok {
			return "", fmt.Errorf("secret '%s' has no field '%s'", secretPath, field)
		}
		str, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("secret '%s' field '%s' is not a string", secretPath, field)
		}
		return str, nil
	}

	if len(values) == 1 {
		for _, value := range values {
			if str, ok := value.(string); ok {
				return str, nil
			}
			return fmt.Sprintf("%v", value), nil
		}
	}

	for _, value := range values {
		if str, ok := value.(string); ok {
			return str, nil
		}
	}

	return "", fmt.Errorf("secret '%s' contains no string values", secretPath)
}

// GetSecrets retrieves multiple secrets from OpenBao
func (o *OpenBaoManager) GetSecrets(projectID string, secretIDs []string) (map[string]string, error) {
	secrets := make(map[string]string)

	for _, secretID := range secretIDs {
		secret, err := o.GetSecret(projectID, secretID)
		if err != nil {
			return nil, fmt.Errorf("failed to get secret '%s': %w", secretID, err)
		}
		secrets[secretID] = secret
	}

	return secrets, nil
}

// GetSecretsByPath retrieves all secrets that start with the given path prefix
func (o *OpenBaoManager) GetSecretsByPath(projectID, secretPath string) (map[string]string, error) {
	secrets := make(map[string]string)

	// List all secrets at the path
	secretNames, err := o.ListSecrets(secretPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets at path '%s': %w", secretPath, err)
	}

	// Get each secret and add it to the result
	for _, secretName := range secretNames {
		// Get the actual secret value
		secretValue, err := o.GetSecret(projectID, secretName)
		if err != nil {
			// Log warning but continue with other secrets
			fmt.Printf("Warning: failed to get secret '%s': %v\n", secretName, err)
			continue
		}

		// Sanitize the secret name for use as an environment variable name
		secrets[secretName] = secretValue
	}

	return secrets, nil
}

// Close closes the OpenBao client
func (o *OpenBaoManager) Close() error {
	// OpenBao client doesn't require explicit closing
	return nil
}

// ListSecrets lists all available secrets in a given path (OpenBao-specific method)
func (o *OpenBaoManager) ListSecrets(path string) ([]string, error) {
	// List secrets at the specified path
	secrets, err := o.client.Logical().List(path)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets at path '%s': %w", path, err)
	}

	if secrets == nil || secrets.Data == nil {
		return []string{}, nil
	}

	// Extract secret names from the response
	var secretNames []string
	if keys, ok := secrets.Data["keys"]; ok {
		if keyList, ok := keys.([]interface{}); ok {
			for _, key := range keyList {
				if keyStr, ok := key.(string); ok {
					secretNames = append(secretNames, keyStr)
				}
			}
		}
	}

	return secretNames, nil
}

// CreateSecret creates a new secret in OpenBao (OpenBao-specific method)
func (o *OpenBaoManager) CreateSecret(path string, data map[string]interface{}) error {
	_, err := o.client.Logical().Write(path, data)
	if err != nil {
		return fmt.Errorf("failed to create secret at path '%s': %w", path, err)
	}

	return nil
}

// UpdateSecret updates an existing secret in OpenBao (OpenBao-specific method)
func (o *OpenBaoManager) UpdateSecret(path string, data map[string]interface{}) error {
	// In OpenBao, writing to an existing path updates the secret
	return o.CreateSecret(path, data)
}

// DeleteSecret deletes a secret from OpenBao (OpenBao-specific method)
func (o *OpenBaoManager) DeleteSecret(path string) error {
	_, err := o.client.Logical().Delete(path)
	if err != nil {
		return fmt.Errorf("failed to delete secret at path '%s': %w", path, err)
	}

	return nil
}
