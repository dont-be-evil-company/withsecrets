package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestLoadSecretsConfig(t *testing.T) {
	// Create a temporary test config file
	testConfig := `---
default:
  provider: gcp
  project: "test-project"
  env:
    TEST_VAR:
      secret-key: "test_secret"
    ANOTHER_VAR:
      secret-key: "another_secret"
      provider: aws
      project: "aws-project"

development:
  provider: gcp
  project: "dev-project"
  env:
    DEV_VAR:
      secret-key: "dev_secret"
`

	tmpFile, err := os.CreateTemp("", "kuba-test-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(testConfig); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}
	tmpFile.Close()

	// Test loading the config
	config, err := LoadSecretsConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify default environment
	defaultEnv, err := config.GetEnvironment("default")
	if err != nil {
		t.Fatalf("Failed to get default environment: %v", err)
	}

	if defaultEnv.Provider != "gcp" {
		t.Errorf("Expected provider 'gcp', got '%s'", defaultEnv.Provider)
	}

	if defaultEnv.Project != "test-project" {
		t.Errorf("Expected project 'test-project', got '%s'", defaultEnv.Project)
	}

	if len(defaultEnv.Env) != 2 {
		t.Errorf("Expected 2 env items, got %d", len(defaultEnv.Env))
	}

	// Verify env map entries
	if defaultEnv.Env["TEST_VAR"].SecretKey != "test_secret" {
		t.Errorf("Expected secret key 'test_secret', got '%s'", defaultEnv.Env["TEST_VAR"].SecretKey)
	}

	if defaultEnv.Env["ANOTHER_VAR"].Provider != "aws" {
		t.Errorf("Expected provider 'aws', got '%s'", defaultEnv.Env["ANOTHER_VAR"].Provider)
	}

	if defaultEnv.Env["ANOTHER_VAR"].Project != "aws-project" {
		t.Errorf("Expected project 'aws-project', got '%s'", defaultEnv.Env["ANOTHER_VAR"].Project)
	}

	// Verify development environment
	devEnv, err := config.GetEnvironment("development")
	if err != nil {
		t.Fatalf("Failed to get development environment: %v", err)
	}

	if devEnv.Provider != "gcp" {
		t.Errorf("Expected provider 'gcp', got '%s'", devEnv.Provider)
	}

	if devEnv.Project != "dev-project" {
		t.Errorf("Expected project 'dev-project', got '%s'", devEnv.Project)
	}
}

func TestGetEnvironmentDefault(t *testing.T) {
	config := &SecretsConfig{
		Environments: map[string]Environment{
			"default": {
				Provider: "gcp",
				Project:  "test-project",
				Env: map[string]EnvItem{
					"TEST_VAR": {SecretKey: "test_secret"},
				},
			},
		},
	}

	// Test getting default environment
	env, err := config.GetEnvironment("")
	if err != nil {
		t.Fatalf("Failed to get default environment: %v", err)
	}

	if env.Provider != "gcp" {
		t.Errorf("Expected provider 'gcp', got '%s'", env.Provider)
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *SecretsConfig
		wantErr bool
	}{
		{
			name: "valid config with secret-key",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "gcp",
						Project:  "test-project",
						Env: map[string]EnvItem{
							"TEST_VAR": {SecretKey: "test_secret"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with value",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "gcp",
						Project:  "test-project",
						Env: map[string]EnvItem{
							"TEST_VAR": {Value: "test_value"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with mixed env items",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "gcp",
						Project:  "test-project",
						Env: map[string]EnvItem{
							"SECRET_VAR": {SecretKey: "test_secret"},
							"VALUE_VAR":  {Value: "test_value"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing provider",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Project: "test-project",
						Env: map[string]EnvItem{
							"TEST_VAR": {SecretKey: "test_secret"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid provider",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "invalid",
						Project:  "test-project",
						Env: map[string]EnvItem{
							"TEST_VAR": {SecretKey: "test_secret"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid local provider without project (value required)",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "local",
						Project:  "",
						Env: map[string]EnvItem{
							"FOO": {Value: "bar"},
							"BAR": {Value: "baz"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing both secret-key and value",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "gcp",
						Project:  "test-project",
						Env: map[string]EnvItem{
							"TEST_VAR": {},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "both secret-key and value specified",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "gcp",
						Project:  "test-project",
						Env: map[string]EnvItem{
							"TEST_VAR": {SecretKey: "test_secret", Value: "test_value"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid AWS config without project",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "aws",
						Project:  "", // Empty project for AWS should be valid
						Env: map[string]EnvItem{
							"AWS_SECRET": {SecretKey: "aws-secret-key"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid Bitwarden config without project",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "bitwarden",
						Project:  "", // Empty project for Bitwarden should be valid
						Env: map[string]EnvItem{
							"BW_SECRET": {SecretKey: "bitwarden-secret-id"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid GCP config without project",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "gcp",
						Project:  "", // Empty project for GCP should be invalid
						Env: map[string]EnvItem{
							"GCP_SECRET": {SecretKey: "gcp-secret-key"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "local provider rejects secret-key",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "local",
						Project:  "",
						Env: map[string]EnvItem{
							"FOO": {SecretKey: "some-secret"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "local provider rejects secret-path",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "local",
						Project:  "",
						Env: map[string]EnvItem{
							"BAR": {SecretPath: []string{"path/to/secrets"}},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid AWS param-key",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "aws",
						Env: map[string]EnvItem{
							"USERNAME": {ParamKey: "/local/withsecrets/config/USERNAME"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid AWS param-path with star",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "aws",
						Env: map[string]EnvItem{
							"*": {ParamPath: []string{"/local/withsecrets/config"}},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid secret-path with star",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "aws",
						Env: map[string]EnvItem{
							"*": {SecretPath: []string{"/local/withsecrets/config"}},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "star with secret-key is invalid",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "aws",
						Env: map[string]EnvItem{
							"*": {SecretKey: "some-secret"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "param-key rejected for non-aws provider",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "gcp",
						Project:  "test-project",
						Env: map[string]EnvItem{
							"FOO": {ParamKey: "/path/to/param"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "param-path rejected for non-aws provider",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "azure",
						Env: map[string]EnvItem{
							"FOO": {ParamPath: []string{"/path/to/params"}},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid openbao field",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "openbao",
						Env: map[string]EnvItem{
							"MY_TOKEN": {SecretKey: "key", Field: "MY_TOKEN"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "openbao field allowed when item provider overrides another environment",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "gcp",
						Project:  "test-project",
						Env: map[string]EnvItem{
							"MY_TOKEN": {SecretKey: "key", Field: "MY_TOKEN", Provider: "openbao"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "field rejected for aws",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "aws",
						Env: map[string]EnvItem{
							"MY_TOKEN": {SecretKey: "key", Field: "MY_TOKEN"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "field rejected for gcp",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "gcp",
						Project:  "test-project",
						Env: map[string]EnvItem{
							"MY_TOKEN": {SecretKey: "key", Field: "MY_TOKEN"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "field rejected when openbao item overrides provider away from openbao",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "openbao",
						Env: map[string]EnvItem{
							"MY_TOKEN": {SecretKey: "key", Field: "MY_TOKEN", Provider: "aws"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "field rejected without secret-key",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "openbao",
						Env: map[string]EnvItem{
							"MY_TOKEN": {Field: "MY_TOKEN", Value: "literal"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "cannot combine secret-key and param-key",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "aws",
						Env: map[string]EnvItem{
							"FOO": {SecretKey: "s", ParamKey: "p"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "local provider rejects param-key",
			config: &SecretsConfig{
				Environments: map[string]Environment{
					"default": {
						Provider: "local",
						Env: map[string]EnvItem{
							"FOO": {ParamKey: "/param"},
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInterpolation(t *testing.T) {
	// Test basic environment variable interpolation
	t.Run("basic env var interpolation", func(t *testing.T) {
		// Set test environment variable
		os.Setenv("TEST_VAR", "test_value")
		defer os.Unsetenv("TEST_VAR")

		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"DB_PASSWORD":          {Value: "secret123"},
						"DB_CONNECTION_STRING": {Value: "postgresql://user:${DB_PASSWORD}@host:5432/db"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that interpolation worked
		env := config.Environments["default"]
		require.Equal(t, "postgresql://user:secret123@host:5432/db", env.Env["DB_CONNECTION_STRING"].Value)
	})

	t.Run("environment variable interpolation", func(t *testing.T) {
		// Set test environment variable
		os.Setenv("DB_HOST", "mydbhost")
		defer os.Unsetenv("DB_HOST")

		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"DB_CONNECTION_STRING": {Value: "postgresql://user:pass@${DB_HOST}:5432/mydb"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that interpolation worked
		env := config.Environments["default"]
		require.Equal(t, "postgresql://user:pass@mydbhost:5432/mydb", env.Env["DB_CONNECTION_STRING"].Value)
	})

	t.Run("mixed interpolation", func(t *testing.T) {
		// Set test environment variable
		os.Setenv("DB_PORT", "5432")
		defer os.Unsetenv("DB_PORT")

		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"DB_PASSWORD":          {Value: "secret123"},
						"DB_HOST":              {Value: "mydbhost"},
						"DB_CONNECTION_STRING": {Value: "postgresql://user:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/mydb"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that interpolation worked
		env := config.Environments["default"]
		require.Equal(t, "postgresql://user:secret123@mydbhost:5432/mydb", env.Env["DB_CONNECTION_STRING"].Value)
	})

	t.Run("no interpolation needed", func(t *testing.T) {
		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"SIMPLE_VALUE": {Value: "no interpolation here"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that value remains unchanged
		env := config.Environments["default"]
		require.Equal(t, "no interpolation here", env.Env["SIMPLE_VALUE"].Value)
	})

	t.Run("unresolved variable remains unchanged", func(t *testing.T) {
		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"UNRESOLVED": {Value: "value with ${UNKNOWN_VAR}"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that unresolved variable remains unchanged
		env := config.Environments["default"]
		require.Equal(t, "value with ${UNKNOWN_VAR}", env.Env["UNRESOLVED"].Value)
	})

	t.Run("numeric values are converted to string", func(t *testing.T) {
		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"PORT": {Value: 8080},
						"URL":  {Value: "http://localhost:${PORT}"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that numeric value was converted and interpolation worked
		env := config.Environments["default"]
		require.Equal(t, "8080", env.Env["PORT"].Value)
		require.Equal(t, "http://localhost:8080", env.Env["URL"].Value)
	})

	t.Run("shell-style default value syntax", func(t *testing.T) {
		// Test with default value when environment variable doesn't exist
		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"APP_ENV":   {Value: "${NODE_ENV:-development}"},
						"REDIS_URL": {Value: "redis://${REDIS_HOST:-localhost}:${REDIS_PORT:-6379}/0"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that default values were used
		env := config.Environments["default"]
		require.Equal(t, "development", env.Env["APP_ENV"].Value)
		require.Equal(t, "redis://localhost:6379/0", env.Env["REDIS_URL"].Value)
	})

	t.Run("shell-style default value with existing env var", func(t *testing.T) {
		// Set test environment variable
		os.Setenv("NODE_ENV", "production")
		defer os.Unsetenv("NODE_ENV")

		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"APP_ENV": {Value: "${NODE_ENV:-development}"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that environment variable value was used instead of default
		env := config.Environments["default"]
		require.Equal(t, "production", env.Env["APP_ENV"].Value)
	})

	t.Run("mixed default value syntax", func(t *testing.T) {
		// Set some environment variables
		os.Setenv("DB_HOST", "mydbhost")
		defer os.Unsetenv("DB_HOST")

		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"DB_PASSWORD":          {Value: "secret123"},
						"DB_CONNECTION_STRING": {Value: "postgresql://user:${DB_PASSWORD}@${DB_HOST:-localhost}:${DB_PORT:-5432}/mydb"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that interpolation worked with mixed syntax
		env := config.Environments["default"]
		require.Equal(t, "postgresql://user:secret123@mydbhost:5432/mydb", env.Env["DB_CONNECTION_STRING"].Value)
	})
}

func TestLoadSecretsConfigWithInterpolation(t *testing.T) {
	// Test loading a config file with interpolation
	t.Run("load config with interpolation", func(t *testing.T) {
		// Set test environment variable
		os.Setenv("DB_HOST", "mydbhost")
		defer os.Unsetenv("DB_HOST")

		configContent := `default:
  provider: gcp
  project: test-project
  env:
    DB_PASSWORD:
      value: "secret123"
    DB_CONNECTION_STRING:
      value: "postgresql://user:${DB_PASSWORD}@${DB_HOST}:5432/mydb"
`
		var config SecretsConfig
		err := yaml.Unmarshal([]byte(configContent), &config)
		require.NoError(t, err)
		err = resolveInheritance(&config)
		require.NoError(t, err)
		err = processValueInterpolations(&config)
		require.NoError(t, err)
		err = validateConfig(&config)
		require.NoError(t, err)

		// Check that interpolation worked
		env := config.Environments["default"]
		require.Equal(t, "postgresql://user:secret123@mydbhost:5432/mydb", env.Env["DB_CONNECTION_STRING"].Value)
	})
}

func TestSecretFieldsInterpolation(t *testing.T) {
	t.Run("interpolate secret-path and secret-key from values", func(t *testing.T) {
		cfg := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"GCP_PROJECT": {Value: "my-proj"},
						"NAME":        {Value: "db-password"},
						"KEYNAME":     {Value: "api-key"},
						"DB_PASSWORD": {SecretPath: []string{"projects/${GCP_PROJECT}/secrets/${NAME}"}},
						"API_KEY":     {SecretKey: "${KEYNAME}"},
					},
				},
			},
		}

		err := resolveInheritance(cfg)
		require.NoError(t, err)
		err = processValueInterpolations(cfg)
		require.NoError(t, err)
		err = validateConfig(cfg)
		require.NoError(t, err)

		env := cfg.Environments["default"]
		require.Equal(t, []string{"projects/my-proj/secrets/db-password"}, env.Env["DB_PASSWORD"].SecretPath)
		require.Equal(t, "api-key", env.Env["API_KEY"].SecretKey)
	})

	t.Run("interpolate with defaults and OS env", func(t *testing.T) {
		// Set one OS env var to verify precedence
		os.Setenv("ORG", "acme")
		defer os.Unsetenv("ORG")

		cfg := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"SERVICE":     {Value: "billing"},
						"SECRET_PATH": {SecretPath: []string{"orgs/${ORG}/svcs/${SERVICE}/secrets/${MISSING:-fallback}"}},
						"SECRET_KEY":  {SecretKey: "${KEY_MISSING:-default-key}"},
					},
				},
			},
		}

		err := resolveInheritance(cfg)
		require.NoError(t, err)
		err = processValueInterpolations(cfg)
		require.NoError(t, err)
		err = validateConfig(cfg)
		require.NoError(t, err)

		env := cfg.Environments["default"]
		require.Equal(t, []string{"orgs/acme/svcs/billing/secrets/fallback"}, env.Env["SECRET_PATH"].SecretPath)
		require.Equal(t, "default-key", env.Env["SECRET_KEY"].SecretKey)
	})

	t.Run("interpolate each path in a list", func(t *testing.T) {
		cfg := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "aws",
					Env: map[string]EnvItem{
						"PREFIX": {Value: "app"},
						"*":      {SecretPath: []string{"${PREFIX}/shared", "${PREFIX}/overrides"}},
					},
				},
			},
		}

		err := resolveInheritance(cfg)
		require.NoError(t, err)
		err = processValueInterpolations(cfg)
		require.NoError(t, err)
		err = validateConfig(cfg)
		require.NoError(t, err)

		require.Equal(t, []string{"app/shared", "app/overrides"}, cfg.Environments["default"].Env["*"].SecretPath)
	})
}

func TestInheritanceLoading(t *testing.T) {
	t.Run("inherits as string", func(t *testing.T) {
		content := `base:
  provider: gcp
  project: p
  env:
    A:
      value: "1"

child:
  provider: gcp
  project: p
  inherits: base
  env:
    B:
      value: "2"
`
		var cfg SecretsConfig
		err := yaml.Unmarshal([]byte(content), &cfg)
		require.NoError(t, err)
		err = resolveInheritance(&cfg)
		require.NoError(t, err)
		err = processValueInterpolations(&cfg)
		require.NoError(t, err)
		err = validateConfig(&cfg)
		require.NoError(t, err)

		env := cfg.Environments["child"]
		require.Len(t, env.Env, 2)
		require.Equal(t, "1", env.Env["A"].Value)
		require.Equal(t, "2", env.Env["B"].Value)
	})

	t.Run("inherits as single-item array", func(t *testing.T) {
		content := `base:
  provider: gcp
  project: p
  env:
    A:
      value: "1"

child:
  provider: gcp
  project: p
  inherits: ["base"]
  env:
    B:
      value: "2"
`
		var cfg SecretsConfig
		err := yaml.Unmarshal([]byte(content), &cfg)
		require.NoError(t, err)
		err = resolveInheritance(&cfg)
		require.NoError(t, err)
		err = processValueInterpolations(&cfg)
		require.NoError(t, err)
		err = validateConfig(&cfg)
		require.NoError(t, err)

		env := cfg.Environments["child"]
		require.Len(t, env.Env, 2)
		require.Equal(t, "1", env.Env["A"].Value)
		require.Equal(t, "2", env.Env["B"].Value)
	})

	t.Run("inherits as two-item array with order and override", func(t *testing.T) {
		content := `base1:
  provider: gcp
  project: p
  env:
    A:
      value: "1"
    COMMON:
      value: "X"

base2:
  provider: gcp
  project: p
  env:
    B:
      value: "2"
    COMMON:
      value: "Y"

child:
  provider: gcp
  project: p
  inherits: ["base1", "base2"]
  env:
    C:
      value: "3"
    COMMON:
      value: "Z"
`
		var cfg SecretsConfig
		err := yaml.Unmarshal([]byte(content), &cfg)
		require.NoError(t, err)
		err = resolveInheritance(&cfg)
		require.NoError(t, err)
		err = processValueInterpolations(&cfg)
		require.NoError(t, err)
		err = validateConfig(&cfg)
		require.NoError(t, err)

		env := cfg.Environments["child"]
		// Should contain A from base1, B from base2, C from child, and COMMON overridden by child
		require.Equal(t, "1", env.Env["A"].Value)
		require.Equal(t, "2", env.Env["B"].Value)
		require.Equal(t, "3", env.Env["C"].Value)
		require.Equal(t, "Z", env.Env["COMMON"].Value)
	})

	t.Run("inherits references unknown environment", func(t *testing.T) {
		content := `base:
  provider: gcp
  project: p
  env:
    A:
      value: "1"

child:
  provider: gcp
  project: p
  inherits: missing
  env:
    B:
      value: "2"
`
		var cfg SecretsConfig
		err := yaml.Unmarshal([]byte(content), &cfg)
		require.NoError(t, err)
		err = resolveInheritance(&cfg)
		require.Error(t, err)
	})
}

func TestMultipleVariableInterpolation(t *testing.T) {
	t.Run("multiple variables in single string", func(t *testing.T) {
		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"VAR1":     {Value: "value1"},
						"VAR2":     {Value: "value2"},
						"VAR3":     {Value: "value3"},
						"COMBINED": {Value: "prefix-${VAR1}-middle-${VAR2}-suffix-${VAR3}"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that all variables were interpolated correctly
		env := config.Environments["default"]
		require.Equal(t, "prefix-value1-middle-value2-suffix-value3", env.Env["COMBINED"].Value)
	})

	t.Run("complex nested interpolation", func(t *testing.T) {
		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"HOST":     {Value: "api.example.com"},
						"PORT":     {Value: "443"},
						"PROTOCOL": {Value: "https"},
						"PATH":     {Value: "/v1/endpoint"},
						"URL":      {Value: "${PROTOCOL}://${HOST}:${PORT}${PATH}"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that complex interpolation worked
		env := config.Environments["default"]
		require.Equal(t, "https://api.example.com:443/v1/endpoint", env.Env["URL"].Value)
	})

	t.Run("multiple variables with defaults", func(t *testing.T) {
		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"EXISTING_VAR": {Value: "existing"},
						"COMPLEX":      {Value: "result: ${EXISTING_VAR}, missing1: ${MISSING1:-default1}, missing2: ${MISSING2:-default2}"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that mixed interpolation with defaults worked
		env := config.Environments["default"]
		require.Equal(t, "result: existing, missing1: default1, missing2: default2", env.Env["COMPLEX"].Value)
	})

	t.Run("variables referencing other variables", func(t *testing.T) {
		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"BASE_URL": {Value: "https://api.example.com"},
						"VERSION":  {Value: "v1"},
						"ENDPOINT": {Value: "users"},
						"FULL_URL": {Value: "${BASE_URL}/${VERSION}/${ENDPOINT}"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that chained interpolation worked
		env := config.Environments["default"]
		require.Equal(t, "https://api.example.com/v1/users", env.Env["FULL_URL"].Value)
	})

	t.Run("multiple iterations needed", func(t *testing.T) {
		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"VAR1": {Value: "first"},
						"VAR2": {Value: "${VAR1}-second"},
						"VAR3": {Value: "${VAR2}-third"},
						"VAR4": {Value: "${VAR3}-fourth"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that chained interpolation worked across multiple iterations
		env := config.Environments["default"]
		require.Equal(t, "first", env.Env["VAR1"].Value)
		require.Equal(t, "first-second", env.Env["VAR2"].Value)
		require.Equal(t, "first-second-third", env.Env["VAR3"].Value)
		require.Equal(t, "first-second-third-fourth", env.Env["VAR4"].Value)
	})

	t.Run("mixed with environment variables", func(t *testing.T) {
		// Set test environment variable
		os.Setenv("EXTERNAL_VAR", "external_value")
		defer os.Unsetenv("EXTERNAL_VAR")

		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"INTERNAL_VAR": {Value: "internal"},
						"MIXED":        {Value: "config: ${INTERNAL_VAR}, env: ${EXTERNAL_VAR}"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that mixed interpolation worked
		env := config.Environments["default"]
		require.Equal(t, "config: internal, env: external_value", env.Env["MIXED"].Value)
	})

	t.Run("deeply nested variable references", func(t *testing.T) {
		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"LEVEL1": {Value: "base"},
						"LEVEL2": {Value: "${LEVEL1}-extended"},
						"LEVEL3": {Value: "${LEVEL2}-more"},
						"LEVEL4": {Value: "${LEVEL3}-final"},
						"RESULT": {Value: "Final result: ${LEVEL4}"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that deeply nested interpolation worked
		env := config.Environments["default"]
		require.Equal(t, "base", env.Env["LEVEL1"].Value)
		require.Equal(t, "base-extended", env.Env["LEVEL2"].Value)
		require.Equal(t, "base-extended-more", env.Env["LEVEL3"].Value)
		require.Equal(t, "base-extended-more-final", env.Env["LEVEL4"].Value)
		require.Equal(t, "Final result: base-extended-more-final", env.Env["RESULT"].Value)
	})

	t.Run("variables with special characters in names", func(t *testing.T) {
		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"VAR_WITH_UNDERSCORES": {Value: "underscore_value"},
						"VAR-WITH-DASHES":      {Value: "dash_value"},
						"VAR.WITH.DOTS":        {Value: "dot_value"},
						"COMBINED":             {Value: "${VAR_WITH_UNDERSCORES}-${VAR-WITH-DASHES}-${VAR.WITH.DOTS}"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that variables with special characters work
		env := config.Environments["default"]
		require.Equal(t, "underscore_value-dash_value-dot_value", env.Env["COMBINED"].Value)
	})

	t.Run("empty and whitespace values", func(t *testing.T) {
		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"EMPTY_VAR": {Value: ""},
						"SPACE_VAR": {Value: " "},
						"RESULT":    {Value: "empty:'${EMPTY_VAR}' space:'${SPACE_VAR}'"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that empty and whitespace values are handled correctly
		env := config.Environments["default"]
		require.Equal(t, "empty:'' space:' '", env.Env["RESULT"].Value)
	})

	t.Run("circular reference detection", func(t *testing.T) {
		config := &SecretsConfig{
			Environments: map[string]Environment{
				"default": {
					Provider: "gcp",
					Project:  "test-project",
					Env: map[string]EnvItem{
						"VAR1": {Value: "${VAR2}"},
						"VAR2": {Value: "${VAR1}"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that circular references are handled gracefully (should remain unchanged)
		env := config.Environments["default"]
		require.Equal(t, "${VAR2}", env.Env["VAR1"].Value)
		require.Equal(t, "${VAR1}", env.Env["VAR2"].Value)
	})

	t.Run("multiple environments with same variable names", func(t *testing.T) {
		config := &SecretsConfig{
			Environments: map[string]Environment{
				"env1": {
					Provider: "gcp",
					Project:  "project1",
					Env: map[string]EnvItem{
						"SHARED_VAR": {Value: "env1_value"},
						"RESULT1":    {Value: "env1: ${SHARED_VAR}"},
					},
				},
				"env2": {
					Provider: "gcp",
					Project:  "project2",
					Env: map[string]EnvItem{
						"SHARED_VAR": {Value: "env2_value"},
						"RESULT2":    {Value: "env2: ${SHARED_VAR}"},
					},
				},
			},
		}

		err := processValueInterpolations(config)
		require.NoError(t, err)

		// Check that each environment resolves its own variables independently
		env1 := config.Environments["env1"]
		env2 := config.Environments["env2"]
		require.Equal(t, "env1: env1_value", env1.Env["RESULT1"].Value)
		require.Equal(t, "env2: env2_value", env2.Env["RESULT2"].Value)
	})
}

func TestEnvItemPathStringOrList(t *testing.T) {
	t.Run("secret-path string", func(t *testing.T) {
		var item EnvItem
		err := yaml.Unmarshal([]byte("secret-path: /a/b\n"), &item)
		require.NoError(t, err)
		require.Equal(t, []string{"/a/b"}, item.SecretPath)
	})

	t.Run("secret-path list", func(t *testing.T) {
		var item EnvItem
		err := yaml.Unmarshal([]byte("secret-path:\n  - /a\n  - /b\n"), &item)
		require.NoError(t, err)
		require.Equal(t, []string{"/a", "/b"}, item.SecretPath)
	})

	t.Run("param-path string", func(t *testing.T) {
		var item EnvItem
		err := yaml.Unmarshal([]byte("param-path: /params\n"), &item)
		require.NoError(t, err)
		require.Equal(t, []string{"/params"}, item.ParamPath)
	})

	t.Run("param-path list skips empty", func(t *testing.T) {
		var item EnvItem
		err := yaml.Unmarshal([]byte("param-path:\n  - /a\n  - \"\"\n  - /b\n"), &item)
		require.NoError(t, err)
		require.Equal(t, []string{"/a", "/b"}, item.ParamPath)
	})

	t.Run("load config with path lists", func(t *testing.T) {
		content := `
default:
  provider: aws
  env:
    "*":
      secret-path:
        - /shared
        - /overrides
    DB:
      param-path: /db
`
		dir := t.TempDir()
		path := dir + "/ws.yaml"
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
		cfg, err := LoadSecretsConfig(path)
		require.NoError(t, err)
		env := cfg.Environments["default"]
		require.Equal(t, []string{"/shared", "/overrides"}, env.Env["*"].SecretPath)
		require.Equal(t, []string{"/db"}, env.Env["DB"].ParamPath)
	})
}

func TestEnvItemUnmarshalsOpenBaoField(t *testing.T) {
	var item EnvItem
	err := yaml.Unmarshal([]byte("secret-key: key\nfield: MY_TOKEN\n"), &item)
	require.NoError(t, err)
	require.Equal(t, "key", item.SecretKey)
	require.Equal(t, "MY_TOKEN", item.Field)
}
