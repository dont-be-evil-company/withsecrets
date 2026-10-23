package guiapi

import (
	"os"
	"testing"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/stretchr/testify/require"
)

const testConfig = `---
default:
  provider: gcp
  project: test-project
  env:
    TEST_VAR:
      value: inline-value
    SECRET_VAR:
      secret-key: my-secret
staging:
  provider: aws
  project: aws-project
  env:
    AWS_VAR:
      secret-path: /path/to/secret
`

func writeTestConfig(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "ws-*.yaml")
	require.NoError(t, err)
	_, err = f.WriteString(testConfig)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

func TestListEnvironments(t *testing.T) {
	path := writeTestConfig(t)
	envs, err := ListEnvironments(path)
	require.NoError(t, err)
	require.Len(t, envs, 2)
	assertEnv(t, envs[0], "default", "gcp", "test-project")
	assertEnv(t, envs[1], "staging", "aws", "aws-project")
}

func assertEnv(t *testing.T, got EnvironmentSummary, name, provider, project string) {
	t.Helper()
	if got.Name != name || got.Provider != provider || got.Project != project {
		t.Fatalf("expected %+v, got %+v", EnvironmentSummary{name, provider, project}, got)
	}
}

func TestListSecretsReturnsPlainAndMaskedValues(t *testing.T) {
	path := writeTestConfig(t)
	rows, err := ListSecrets(t.Context(), path, "default")
	require.NoError(t, err)
	require.Len(t, rows, 2)

	var inline SecretRow
	for _, row := range rows {
		if row.EnvVar == "TEST_VAR" {
			inline = row
			break
		}
	}
	require.Equal(t, "inline-value", inline.Value)
	require.Equal(t, MaskValue("inline-value"), inline.MaskedValue)
}

func TestUpdateSecretRejectsNonSecretKey(t *testing.T) {
	path := writeTestConfig(t)
	err := UpdateSecret(t.Context(), path, "default", "TEST_VAR", "new")
	require.Error(t, err)
	require.Contains(t, err.Error(), "value")
}

func TestDeleteSecretRejectsNonSecretKey(t *testing.T) {
	path := writeTestConfig(t)
	err := DeleteSecret(t.Context(), path, "default", "TEST_VAR")
	require.Error(t, err)
	require.Contains(t, err.Error(), "value")
}

func TestCreateSecretRequiresFields(t *testing.T) {
	path := writeTestConfig(t)
	err := CreateSecret(t.Context(), CreateInput{
		ConfigPath: path,
		EnvName:    "default",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "required")
}

func TestPathMappingCRUD(t *testing.T) {
	path := writeTestConfig(t)

	err := CreateSecret(t.Context(), CreateInput{
		ConfigPath: path,
		EnvName:    "staging",
		EnvVar:     "*",
		Kind:       "param-path",
		Paths:      []string{"/a", "/b"},
	})
	require.NoError(t, err)

	cfg, err := config.LoadSecretsConfig(path)
	require.NoError(t, err)
	require.Equal(t, []string{"/a", "/b"}, cfg.Environments["staging"].Env["*"].ParamPath)

	err = UpdatePathMapping(t.Context(), path, "staging", "*", []string{"/c"})
	require.NoError(t, err)
	cfg, err = config.LoadSecretsConfig(path)
	require.NoError(t, err)
	require.Equal(t, []string{"/c"}, cfg.Environments["staging"].Env["*"].ParamPath)

	err = DeleteSecret(t.Context(), path, "staging", "*")
	require.NoError(t, err)
	cfg, err = config.LoadSecretsConfig(path)
	require.NoError(t, err)
	_, exists := cfg.Environments["staging"].Env["*"]
	require.False(t, exists)
}

func TestCreatePathMappingRejectsEmpty(t *testing.T) {
	path := writeTestConfig(t)
	err := CreateSecret(t.Context(), CreateInput{
		ConfigPath: path,
		EnvName:    "staging",
		EnvVar:     "DB",
		Kind:       "secret-path",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "path")
}

func TestUpdatePathMappingRejectsSecretKey(t *testing.T) {
	path := writeTestConfig(t)
	err := UpdatePathMapping(t.Context(), path, "default", "SECRET_VAR", []string{"/a"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "secret-path")
}
