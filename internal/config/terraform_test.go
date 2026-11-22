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
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "iac", "default"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "iac", "child"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "iac", "default", "cloudrun.tf"), []byte(terraformFixture), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "iac", "child", "extra.tf"), []byte(`
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
  path: ./iac/default/cloudrun.tf
  env:
    NODE_ENV:
      value: development
child:
  provider: terraform
  path: ./iac/child/extra.tf
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

const moduleMainTF = `
variable "app" {
  type = string
}

variable "environment" {
  type = string
}

variable "env" {
  type    = map(string)
  default = {}
}

variable "secrets" {
  type    = map(string)
  default = {}
}

locals {
  env_plain = merge(
    {
      APP_NAME    = var.app
      APP_ENV     = lookup({ prod = "production", dev = "development" }, var.environment, var.environment)
      APP_DEBUG   = var.environment == "prod" ? "false" : "true"
      LOG_CHANNEL = "stderr"
      LOG_LEVEL   = var.environment == "prod" ? "error" : "debug"
    },
    var.env,
  )
  env_secrets = var.secrets
}

resource "google_cloud_run_v2_service" "service" {
  template {
    containers {
      dynamic "env" {
        for_each = local.env_plain
        iterator = plain
        content {
          name  = plain.key
          value = plain.value
        }
      }

      dynamic "env" {
        for_each = local.env_secrets
        iterator = secret
        content {
          name = secret.key
          value_source {
            secret_key_ref {
              secret  = secret.value
              version = "latest"
            }
          }
        }
      }
    }
  }
}
`

const appMainTF = `
variable "environment" {
  type = string
}

variable "project" {
  type = string
}

variable "workers" {
  type    = number
  default = 5
}

locals {
  secret_env = upper(var.environment)
}

module "remote" {
  source = "hashicorp/google/google"
}

module "app" {
  source = "../module"

  app         = "pdf"
  environment = var.environment

  env = {
    APP_NAME                = "PDF Generator"
    SESSION_DRIVER          = "cookie"
    GOOGLE_CLOUD_PROJECT_ID = var.project
    WORKERS                 = tostring(var.workers)
  }

  secrets = {
    APP_KEY = "APP_KEY"
    DB_HOST = "WERP_${local.secret_env}_DB_HOST"
    DB_SKIP = "WERP_${var.missing}_DB"
  }
}
`

func writeModuleFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "module"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "app", "variables"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module", "main.tf"), []byte(moduleMainTF), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app", "main.tf"), []byte(appMainTF), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app", "variables", "dev.tfvars"), []byte("environment = \"dev\"\nworkers = 7\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app", "variables", "prod.tfvars"), []byte("environment = \"prod\"\nworkers = 50\n"), 0o600))
	return dir
}

func TestLoadTerraformModule(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "from-env")
	t.Setenv("GCLOUD_PROJECT", "")
	t.Setenv("CLOUDSDK_CORE_PROJECT", "")
	t.Setenv("CLOUDSDK_CONFIG", t.TempDir())
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")

	dir := writeModuleFixture(t)
	ws := []byte(`plain:
  provider: terraform
  path: ./app/main.tf
  project: from-yaml
dev:
  provider: terraform
  path: ./app/main.tf
  var-file: ./app/variables/dev.tfvars
  project: from-yaml
  env:
    APP_ENV:
      value: local
prod:
  provider: terraform
  path: ./app/main.tf
  var-file: ./app/variables/prod.tfvars
  project: from-yaml
module:
  provider: terraform
  path: ./module/main.tf
`)
	configPath := filepath.Join(dir, "ws.yaml")
	require.NoError(t, os.WriteFile(configPath, ws, 0o600))

	cfg, err := LoadSecretsConfig(configPath)
	require.NoError(t, err)

	plain := cfg.Environments["plain"]
	require.Equal(t, "PDF Generator", plain.Env["APP_NAME"].Value)
	require.Equal(t, "cookie", plain.Env["SESSION_DRIVER"].Value)
	require.Equal(t, "from-yaml", plain.Env["GOOGLE_CLOUD_PROJECT_ID"].Value)
	require.Equal(t, "5", plain.Env["WORKERS"].Value)
	require.Equal(t, "stderr", plain.Env["LOG_CHANNEL"].Value)
	require.Equal(t, "APP_KEY", plain.Env["APP_KEY"].SecretKey)
	require.Equal(t, "gcp", plain.Env["APP_KEY"].Provider)
	require.Equal(t, "from-yaml", plain.Env["APP_KEY"].Project)
	require.NotContains(t, plain.Env, "DB_HOST")
	require.NotContains(t, plain.Env, "DB_SKIP")
	require.NotContains(t, plain.Env, "APP_ENV")
	require.NotContains(t, plain.Env, "APP_DEBUG")
	require.NotContains(t, plain.Env, "LOG_LEVEL")

	dev := cfg.Environments["dev"]
	require.Equal(t, "local", dev.Env["APP_ENV"].Value)
	require.Equal(t, "true", dev.Env["APP_DEBUG"].Value)
	require.Equal(t, "debug", dev.Env["LOG_LEVEL"].Value)
	require.Equal(t, "stderr", dev.Env["LOG_CHANNEL"].Value)
	require.Equal(t, "PDF Generator", dev.Env["APP_NAME"].Value)
	require.Equal(t, "7", dev.Env["WORKERS"].Value)
	require.Equal(t, "from-yaml", dev.Env["GOOGLE_CLOUD_PROJECT_ID"].Value)
	require.Equal(t, EnvItem{SecretKey: "APP_KEY", Provider: "gcp", Project: "from-yaml"}, dev.Env["APP_KEY"])
	require.Equal(t, EnvItem{SecretKey: "WERP_DEV_DB_HOST", Provider: "gcp", Project: "from-yaml"}, dev.Env["DB_HOST"])
	require.NotContains(t, dev.Env, "DB_SKIP")

	prod := cfg.Environments["prod"]
	require.Equal(t, "production", prod.Env["APP_ENV"].Value)
	require.Equal(t, "false", prod.Env["APP_DEBUG"].Value)
	require.Equal(t, "error", prod.Env["LOG_LEVEL"].Value)
	require.Equal(t, "50", prod.Env["WORKERS"].Value)
	require.Equal(t, "WERP_PROD_DB_HOST", prod.Env["DB_HOST"].SecretKey)

	module := cfg.Environments["module"]
	require.Equal(t, "stderr", module.Env["LOG_CHANNEL"].Value)
	require.NotContains(t, module.Env, "APP_NAME")
	require.NotContains(t, module.Env, "APP_ENV")
	require.NotContains(t, module.Env, "APP_KEY")
}

func TestLoadTerraformVarFileIsRequiredWhenSet(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`
resource "google_cloud_run_v2_service" "service" {
  template {
    containers {
      env {
        name  = "LOG_CHANNEL"
        value = "stderr"
      }
    }
  }
}
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ws.yaml"), []byte(`default:
  provider: terraform
  path: ./main.tf
  var-file: ./missing.tfvars
`), 0o600))

	_, err := LoadSecretsConfig(filepath.Join(dir, "ws.yaml"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing.tfvars")
}

func TestLoadTerraformMissingModule(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "app"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app", "main.tf"), []byte(`
module "app" {
  source = "../missing"
}
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ws.yaml"), []byte(`default:
  provider: terraform
  path: ./app/main.tf
  env:
    LOG_CHANNEL:
      value: stderr
`), 0o600))

	_, err := LoadSecretsConfig(filepath.Join(dir, "ws.yaml"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing")
}

func TestTerraformDynamicEnvDefaultIterator(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`
resource "google_cloud_run_v2_service" "service" {
  template {
    containers {
      dynamic "env" {
        for_each = { FROM_EACH = "each" }
        content {
          name  = each.key
          value = each.value
        }
      }
      dynamic "env" {
        for_each = { FROM_LABEL = "label" }
        content {
          name  = env.key
          value = env.value
        }
      }
    }
  }
}
`), 0o600))

	items, err := terraformEnvFromFile(filepath.Join(dir, "main.tf"), "", "")
	require.NoError(t, err)
	require.Equal(t, "each", items["FROM_EACH"].Value)
	require.Equal(t, "label", items["FROM_LABEL"].Value)
}
