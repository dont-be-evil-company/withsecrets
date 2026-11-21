package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const terraformFixture = `
resource "google_cloud_run_v2_service" "default" {
  template {
    containers {
      env {
        name  = "NODE_ENV"
        value = "production"
      }
      env {
        name  = "GCP_PROJECT"
        value = var.project
      }
      env {
        name  = "DB_NAME"
        value = var.environment != "prod" ? "defshop_one_dev" : "defshop_one"
      }
      env {
        name = "AUTH_SECRET"
        value_source {
          secret_key_ref {
            secret  = "auth-secret"
            version = "latest"
          }
        }
      }
      env {
        name = "DB_CONNECTION_STRING"
        value_source {
          secret_key_ref {
            secret  = "db-url-${var.environment}"
            version = "latest"
          }
        }
      }
      env {
        name = "FULL_SECRET"
        value_source {
          secret_key_ref {
            secret  = "projects/from-resource/secrets/full-secret/versions/latest"
            version = "latest"
          }
        }
      }
    }
  }
}

resource "google_cloud_run_v2_service" "later" {
  template {
    containers {
      env {
        name  = "NODE_ENV"
        value = "from-second"
      }
    }
  }
}

resource "google_cloudfunctions2_function" "fn" {
  service_config {
    environment_variables = {
      FN_PLAIN = "plain"
      FN_EXPR  = var.missing
    }
    secret_environment_variables {
      key        = "FN_SECRET"
      project_id = "fn-project"
      secret     = "fn-secret"
      version    = "latest"
    }
    secret_environment_variables {
      key    = "FN_EXPR_SECRET"
      secret = "fn-${var.environment}"
    }
  }
}
`

func TestTerraformEnvItems(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "from-env")
	t.Setenv("GCLOUD_PROJECT", "")
	t.Setenv("CLOUDSDK_CORE_PROJECT", "")

	items, err := terraformEnvItems([]byte(terraformFixture), "cloudrun.tf", "")
	require.NoError(t, err)

	require.Equal(t, "from-second", items["NODE_ENV"].Value)
	require.Equal(t, "plain", items["FN_PLAIN"].Value)

	require.NotContains(t, items, "GCP_PROJECT")
	require.NotContains(t, items, "DB_NAME")
	require.NotContains(t, items, "DB_CONNECTION_STRING")
	require.NotContains(t, items, "FN_EXPR")
	require.NotContains(t, items, "FN_EXPR_SECRET")

	require.Equal(t, EnvItem{SecretKey: "auth-secret", Provider: "gcp", Project: "from-env"}, items["AUTH_SECRET"])
	require.Equal(t, EnvItem{SecretKey: "full-secret", Provider: "gcp", Project: "from-resource"}, items["FULL_SECRET"])
	require.Equal(t, EnvItem{SecretKey: "fn-secret", Provider: "gcp", Project: "fn-project"}, items["FN_SECRET"])
}

func TestTerraformProjectFallback(t *testing.T) {
	secretOnly := []byte(`
resource "google_cloud_run_v2_service" "default" {
  template {
    containers {
      env {
        name = "AUTH_SECRET"
        value_source {
          secret_key_ref {
            secret = "auth-secret"
          }
        }
      }
    }
  }
}
`)

	t.Run("environment project wins over the environment variable", func(t *testing.T) {
		t.Setenv("GOOGLE_CLOUD_PROJECT", "from-env")
		items, err := terraformEnvItems(secretOnly, "cloudrun.tf", "from-ws")
		require.NoError(t, err)
		require.Equal(t, "from-ws", items["AUTH_SECRET"].Project)
	})

	t.Run("gcloud config is used when nothing else is set", func(t *testing.T) {
		t.Setenv("GOOGLE_CLOUD_PROJECT", "")
		t.Setenv("GCLOUD_PROJECT", "")
		t.Setenv("CLOUDSDK_CORE_PROJECT", "")
		configDir := t.TempDir()
		t.Setenv("CLOUDSDK_CONFIG", configDir)
		t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
		require.NoError(t, os.WriteFile(filepath.Join(configDir, "active_config"), []byte("dev\n"), 0o600))
		require.NoError(t, os.MkdirAll(filepath.Join(configDir, "configurations"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(configDir, "configurations", "config_dev"), []byte("[core]\nproject = from-gcloud\n"), 0o600))

		items, err := terraformEnvItems(secretOnly, "cloudrun.tf", "")
		require.NoError(t, err)
		require.Equal(t, "from-gcloud", items["AUTH_SECRET"].Project)
	})

	t.Run("missing project is an error", func(t *testing.T) {
		t.Setenv("GOOGLE_CLOUD_PROJECT", "")
		t.Setenv("GCLOUD_PROJECT", "")
		t.Setenv("CLOUDSDK_CORE_PROJECT", "")
		t.Setenv("CLOUDSDK_CONFIG", t.TempDir())
		t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")

		_, err := terraformEnvItems(secretOnly, "cloudrun.tf", "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "AUTH_SECRET")
	})
}

func TestLoadTerraformConfig(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "from-env")
	t.Setenv("GCLOUD_PROJECT", "")
	t.Setenv("CLOUDSDK_CORE_PROJECT", "")

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "iac"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "iac", "cloudrun.tf"), []byte(terraformFixture), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "iac", "extra.tf"), []byte(`
resource "google_cloud_run_v2_service" "extra" {
  template {
    containers {
      env {
        name  = "EXTRA"
        value = "from-child-file"
      }
    }
  }
}
`), 0o600))

	ws := []byte(`default:
  provider: terraform
  path: ./iac/cloudrun.tf
  env:
    NODE_ENV:
      value: development
child:
  provider: terraform
  path: ./iac/extra.tf
  inherits: default
  env:
    FN_PLAIN:
      value: overridden
`)
	configPath := filepath.Join(dir, "ws.yaml")
	require.NoError(t, os.WriteFile(configPath, ws, 0o600))

	cfg, err := LoadSecretsConfig(configPath)
	require.NoError(t, err)

	def := cfg.Environments["default"]
	require.Equal(t, "development", def.Env["NODE_ENV"].Value)
	require.Equal(t, "plain", def.Env["FN_PLAIN"].Value)
	require.Equal(t, "auth-secret", def.Env["AUTH_SECRET"].SecretKey)
	require.Equal(t, "gcp", def.Env["AUTH_SECRET"].Provider)
	require.Equal(t, "from-env", def.Env["AUTH_SECRET"].Project)

	child := cfg.Environments["child"]
	require.Equal(t, "development", child.Env["NODE_ENV"].Value)
	require.Equal(t, "overridden", child.Env["FN_PLAIN"].Value)
	require.Equal(t, "from-child-file", child.Env["EXTRA"].Value)
	require.Equal(t, "auth-secret", child.Env["AUTH_SECRET"].SecretKey)
}

func TestLoadTerraformConfigRejectsMissingFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ws.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`default:
  provider: terraform
  path: ./iac/missing.tf
`), 0o600))

	_, err := LoadSecretsConfig(configPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing.tf")
}

func TestTerraformValidation(t *testing.T) {
	t.Run("path is required", func(t *testing.T) {
		cfg := &SecretsConfig{Environments: map[string]Environment{
			"default": {Provider: "terraform", Env: map[string]EnvItem{"NODE_ENV": {Value: "production"}}},
		}}
		err := validateConfig(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "path is required")
	})

	t.Run("project is optional and value-only entries are allowed", func(t *testing.T) {
		cfg := &SecretsConfig{Environments: map[string]Environment{
			"default": {
				Provider: "terraform",
				Path:     "./iac/cloudrun.tf",
				Env:      map[string]EnvItem{"NODE_ENV": {Value: "production"}},
			},
		}}
		require.NoError(t, validateConfig(cfg))
	})

	t.Run("secret-key is rejected on a terraform entry", func(t *testing.T) {
		cfg := &SecretsConfig{Environments: map[string]Environment{
			"default": {
				Provider: "terraform",
				Path:     "./iac/cloudrun.tf",
				Env:      map[string]EnvItem{"AUTH_SECRET": {SecretKey: "auth-secret"}},
			},
		}}
		err := validateConfig(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "requires 'value'")
	})

	t.Run("empty env after expansion still fails", func(t *testing.T) {
		cfg := &SecretsConfig{Environments: map[string]Environment{
			"default": {Provider: "terraform", Path: "./iac/cloudrun.tf"},
		}}
		err := validateConfig(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "at least one env item")
	})
}
