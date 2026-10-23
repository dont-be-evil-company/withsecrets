package secrets

import (
	"context"
	"fmt"
	"testing"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
)

type fieldProvider struct {
	fields          map[string]map[string]string
	getSecretsCalls int
	fieldCalls      []string
}

func (f *fieldProvider) GetSecret(_, secretID string) (string, error) {
	return "", fmt.Errorf("unexpected GetSecret for %s", secretID)
}

func (f *fieldProvider) GetSecrets(_ string, _ []string) (map[string]string, error) {
	f.getSecretsCalls++
	return nil, fmt.Errorf("unexpected GetSecrets")
}

func (f *fieldProvider) GetSecretsByPath(_, _ string) (map[string]string, error) {
	return nil, fmt.Errorf("unexpected GetSecretsByPath")
}

func (f *fieldProvider) Close() error { return nil }

func (f *fieldProvider) GetSecretField(_, secretID, field string) (string, error) {
	f.fieldCalls = append(f.fieldCalls, secretID+"/"+field)
	values, ok := f.fields[secretID]
	if !ok {
		return "", fmt.Errorf("missing secret %s", secretID)
	}
	value, ok := values[field]
	if !ok {
		return "", fmt.Errorf("missing field %s", field)
	}
	return value, nil
}

func TestOpenBaoFieldSelectionsStayDistinct(t *testing.T) {
	fake := &fieldProvider{
		fields: map[string]map[string]string{
			"key": {
				"MY_TOKEN": "tok",
				"OTHER":    "other",
			},
		},
	}
	factory := &SecretManagerFactory{
		createManager: func(context.Context, string, string, string) (SecretManager, error) {
			return fake, nil
		},
	}

	env := &config.Environment{
		Provider: "openbao",
		Project:  "kv/data/shared",
		Env: map[string]config.EnvItem{
			"MY_TOKEN": {SecretKey: "key", Field: "MY_TOKEN"},
			"OTHER":    {SecretKey: "key", Field: "OTHER"},
		},
	}

	got, _, err := factory.GetSecretsForEnvironmentWithCache(context.Background(), env, "", "")
	if err != nil {
		t.Fatalf("GetSecretsForEnvironmentWithCache: %v", err)
	}
	if got["MY_TOKEN"] != "tok" || got["OTHER"] != "other" {
		t.Fatalf("got %#v, want distinct field values", got)
	}
	if fake.getSecretsCalls != 0 {
		t.Fatalf("field selections were batched into GetSecrets %d times", fake.getSecretsCalls)
	}
	if len(fake.fieldCalls) != 2 {
		t.Fatalf("field calls = %#v, want 2", fake.fieldCalls)
	}
}
