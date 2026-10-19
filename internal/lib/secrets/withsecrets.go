package secrets

import "fmt"

// withSecretsManager is the opt-in hosted vault provider.
// It does not decrypt vault items and it does not contact a server during
// construction. Unattended decryption needs a future scoped machine grant.
type withSecretsManager struct{}

func NewWithSecretsManager() (SecretManager, error) {
	return &withSecretsManager{}, nil
}

func (m *withSecretsManager) GetSecret(_, _ string) (string, error) {
	return "", errWithSecretsHeadless
}

func (m *withSecretsManager) GetSecrets(_ string, _ []string) (map[string]string, error) {
	return nil, errWithSecretsHeadless
}

func (m *withSecretsManager) GetSecretsByPath(_, _ string) (map[string]string, error) {
	return nil, errWithSecretsHeadless
}

func (m *withSecretsManager) Close() error { return nil }

var errWithSecretsHeadless = fmt.Errorf("withsecrets vault provider does not decrypt items in unattended mode; configure a scoped machine grant instead of an account master key")
