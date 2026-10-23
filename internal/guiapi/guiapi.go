package guiapi

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/secrets"
)

var errUnexpectedGCPManager = fmt.Errorf("unexpected gcp secret manager type")

// ListEnvironments returns all configured environment names with provider/project.
func ListEnvironments(configPath string) ([]EnvironmentSummary, error) {
	cfg, err := config.LoadSecretsConfig(configPath)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(cfg.Environments))
	for name := range cfg.Environments {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]EnvironmentSummary, 0, len(names))
	for _, name := range names {
		env := cfg.Environments[name]
		out = append(out, EnvironmentSummary{
			Name:     name,
			Provider: env.Provider,
			Project:  env.Project,
		})
	}
	return out, nil
}

// ListSecrets returns secret rows for an environment with plain and masked values.
func ListSecrets(ctx context.Context, configPath, envName string) ([]SecretRow, error) {
	cfg, err := config.LoadSecretsConfig(configPath)
	if err != nil {
		return nil, err
	}
	env, err := cfg.GetEnvironment(envName)
	if err != nil {
		return nil, err
	}

	factory := secrets.NewSecretManagerFactory()
	values, refs, err := factory.GetSecretsForEnvironmentWithCache(ctx, env, configPath, envName)
	if err != nil {
		return nil, err
	}

	items := env.GetEnvItems()
	rows := make([]SecretRow, 0, len(items))
	for _, it := range items {
		provider := it.Provider
		if provider == "" {
			provider = env.Provider
		}
		project := it.Project
		if project == "" {
			project = env.Project
		}
		region := it.Region
		if region == "" {
			region = env.Region
		}

		refKind := "value"
		ref := ""
		var paths []string
		if it.SecretKey != "" {
			refKind = "secret-key"
			ref = it.SecretKey
		} else if len(it.SecretPath) > 0 {
			refKind = "secret-path"
			paths = append([]string(nil), it.SecretPath...)
			ref = config.FormatPathRef(paths)
		} else if it.ParamKey != "" {
			refKind = "param-key"
			ref = it.ParamKey
		} else if len(it.ParamPath) > 0 {
			refKind = "param-path"
			paths = append([]string(nil), it.ParamPath...)
			ref = config.FormatPathRef(paths)
		}

		val := values[it.EnvironmentVariable]

		rows = append(rows, SecretRow{
			EnvVar:      it.EnvironmentVariable,
			Value:       val,
			MaskedValue: MaskValue(val),
			RefKind:     refKind,
			Ref:         ref,
			Paths:       paths,
			ProviderKey: refs[it.EnvironmentVariable],
			Provider:    provider,
			Project:     project,
			Region:      region,
			IsMapping:   true,
		})

		// For path / "*" bulk mappings, also surface expanded env vars
		if len(it.SecretPath) > 0 || len(it.ParamPath) > 0 {
			prefix := ""
			if it.EnvironmentVariable != "*" {
				prefix = it.EnvironmentVariable + "_"
			}
			for k, v := range values {
				include := false
				if it.EnvironmentVariable == "*" {
					if _, isMapped := env.Env[k]; !isMapped {
						include = true
					}
				} else if strings.HasPrefix(k, prefix) && k != it.EnvironmentVariable {
					include = true
				}
				if !include {
					continue
				}
				rows = append(rows, SecretRow{
					EnvVar:      k,
					Value:       v,
					MaskedValue: MaskValue(v),
					RefKind:     refKind,
					Ref:         ref,
					Paths:       paths,
					ProviderKey: refs[k],
					Provider:    provider,
					Project:     project,
					Region:      region,
					IsMapping:   false,
				})
			}
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].EnvVar < rows[j].EnvVar })
	return rows, nil
}

// GetSecret returns the unmasked value for a single env var.
func GetSecret(ctx context.Context, configPath, envName, envVar string) (string, error) {
	rows, err := ListSecrets(ctx, configPath, envName)
	if err != nil {
		return "", err
	}
	for _, r := range rows {
		if r.EnvVar == envVar {
			return r.Value, nil
		}
	}
	return "", fmt.Errorf("secret '%s' not found in environment '%s'", envVar, envName)
}

// CreateSecret creates a provider secret and adds a ws.yaml mapping.
func CreateSecret(ctx context.Context, in CreateInput) error {
	envVar := strings.TrimSpace(in.EnvVar)
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = "secret-key"
	}

	switch kind {
	case "secret-path", "param-path":
		return createPathMapping(in.ConfigPath, in.EnvName, envVar, kind, in.Paths)
	case "secret-key":
		// continue below
	default:
		return fmt.Errorf("unsupported mapping kind %q", kind)
	}

	secretKey := strings.TrimSpace(in.SecretKey)
	if envVar == "" || secretKey == "" {
		return fmt.Errorf("env var and secret key are required")
	}
	if envVar == "*" {
		return fmt.Errorf(`"*" is only valid with secret-path or param-path`)
	}

	cfg, err := config.LoadSecretsConfig(in.ConfigPath)
	if err != nil {
		return err
	}
	env, err := cfg.GetEnvironment(in.EnvName)
	if err != nil {
		return err
	}

	factory := secrets.NewSecretManagerFactory()
	sm, err := factory.CreateSecretManager(ctx, env.Provider, env.Project, env.Region)
	if err != nil {
		return err
	}
	defer sm.Close()

	if env.Provider == "gcp" {
		if gcpSM, ok := sm.(*secrets.GCPSecretManager); ok {
			if in.Replication == "user-managed" && len(in.Locations) > 0 {
				gcpSM.SetCreateLocations(in.Locations)
			} else {
				gcpSM.SetCreateLocations(nil)
			}
		}
	}

	mut, err := secrets.AsMutator(sm)
	if err != nil {
		return err
	}

	desc := strings.TrimSpace(in.Description)
	if err := mut.CreateSecret(secretKey, in.Value, desc); err != nil {
		return err
	}

	if err := config.AddOrUpdateEnvSecretKeyMapping(in.ConfigPath, in.EnvName, envVar, secretKey); err != nil {
		return err
	}
	cacheSetSecret(in.ConfigPath, in.EnvName, envVar, in.Value)
	return nil
}

func createPathMapping(configPath, envName, envVar, kind string, paths []string) error {
	if strings.TrimSpace(envVar) == "" {
		return fmt.Errorf("env var is required")
	}
	cleaned := config.NormalizePaths(paths)
	if len(cleaned) == 0 {
		return fmt.Errorf("at least one path is required")
	}

	cfg, err := config.LoadSecretsConfig(configPath)
	if err != nil {
		return err
	}
	env, err := cfg.GetEnvironment(envName)
	if err != nil {
		return err
	}
	if kind == "param-path" && env.Provider != "aws" {
		return fmt.Errorf("param-path is only supported with provider 'aws'")
	}
	if env.Provider == "local" {
		return fmt.Errorf("provider 'local' does not support path mappings")
	}

	if err := config.AddOrUpdateEnvPathMapping(configPath, envName, envVar, kind, cleaned); err != nil {
		return err
	}
	cacheClearEnv(configPath, envName)
	return nil
}

func isPathKind(kind string) bool {
	return kind == "secret-path" || kind == "param-path"
}

func mappingKind(configPath, envName, envVar string) (string, error) {
	cfg, err := config.LoadSecretsConfig(configPath)
	if err != nil {
		return "", err
	}
	env, err := cfg.GetEnvironment(envName)
	if err != nil {
		return "", err
	}
	item, ok := env.Env[envVar]
	if !ok {
		return "", fmt.Errorf("secret '%s' not found in environment '%s'", envVar, envName)
	}
	switch {
	case item.SecretKey != "":
		return "secret-key", nil
	case len(item.SecretPath) > 0:
		return "secret-path", nil
	case item.ParamKey != "":
		return "param-key", nil
	case len(item.ParamPath) > 0:
		return "param-path", nil
	case item.Value != nil:
		return "value", nil
	default:
		return "", fmt.Errorf("secret '%s' not found in environment '%s'", envVar, envName)
	}
}

// UpdateSecret updates a provider secret or parameter value.
func UpdateSecret(ctx context.Context, configPath, envName, envVar, newValue string) error {
	row, err := findSecretRow(ctx, configPath, envName, envVar)
	if err != nil {
		return err
	}
	if row.IsMapping && isPathKind(row.RefKind) {
		return fmt.Errorf("edit paths on the %s mapping, not its value", row.RefKind)
	}
	if row.RefKind == "value" {
		return fmt.Errorf("edit is not supported for static value mappings")
	}

	key := row.ProviderKey
	if key == "" {
		key = row.Ref
	}
	if key == "" {
		return fmt.Errorf("cannot determine provider key for '%s'", envVar)
	}

	factory := secrets.NewSecretManagerFactory()
	sm, err := factory.CreateSecretManager(ctx, row.Provider, row.Project, row.Region)
	if err != nil {
		return err
	}
	defer sm.Close()

	if row.RefKind == "param-key" || row.RefKind == "param-path" {
		paramStore, ok := sm.(secrets.ParameterStore)
		if !ok {
			return fmt.Errorf("provider %s does not support Parameter Store updates", row.Provider)
		}
		if err := paramStore.PutParameter(row.Project, key, newValue); err != nil {
			return err
		}
		cacheSetSecret(configPath, envName, envVar, newValue)
		return nil
	}

	mut, err := secrets.AsMutator(sm)
	if err != nil {
		return err
	}
	if err := mut.UpdateSecret(key, newValue); err != nil {
		return err
	}
	cacheSetSecret(configPath, envName, envVar, newValue)
	return nil
}

// UpdatePathMapping replaces secret-path or param-path values in ws.yaml.
func UpdatePathMapping(_ context.Context, configPath, envName, envVar string, paths []string) error {
	kind, err := mappingKind(configPath, envName, envVar)
	if err != nil {
		return err
	}
	if !isPathKind(kind) {
		return fmt.Errorf("path update is only supported for secret-path and param-path mappings")
	}
	cleaned := config.NormalizePaths(paths)
	if len(cleaned) == 0 {
		return fmt.Errorf("at least one path is required")
	}
	if err := config.AddOrUpdateEnvPathMapping(configPath, envName, envVar, kind, cleaned); err != nil {
		return err
	}
	cacheClearEnv(configPath, envName)
	return nil
}

// DeleteSecret deletes a provider secret/parameter and, for mapping rows, the ws.yaml mapping.
func DeleteSecret(ctx context.Context, configPath, envName, envVar string) error {
	if kind, err := mappingKind(configPath, envName, envVar); err == nil && isPathKind(kind) {
		if err := config.RemoveEnvMapping(configPath, envName, envVar); err != nil {
			return err
		}
		cacheClearEnv(configPath, envName)
		return nil
	}

	row, err := findSecretRow(ctx, configPath, envName, envVar)
	if err != nil {
		return err
	}
	if row.RefKind == "value" {
		return fmt.Errorf("delete is not supported for static value mappings")
	}

	key := row.ProviderKey
	if key == "" {
		key = row.Ref
	}
	if key == "" {
		return fmt.Errorf("cannot determine provider key for '%s'", envVar)
	}

	factory := secrets.NewSecretManagerFactory()
	sm, err := factory.CreateSecretManager(ctx, row.Provider, row.Project, row.Region)
	if err != nil {
		return err
	}
	defer sm.Close()

	if row.RefKind == "param-key" || row.RefKind == "param-path" {
		paramStore, ok := sm.(secrets.ParameterStore)
		if !ok {
			return fmt.Errorf("provider %s does not support Parameter Store deletes", row.Provider)
		}
		if err := paramStore.DeleteParameter(row.Project, key); err != nil {
			return err
		}
	} else {
		mut, err := secrets.AsMutator(sm)
		if err != nil {
			return err
		}
		if err := mut.DeleteSecret(key, true); err != nil {
			return err
		}
	}

	if row.IsMapping {
		if err := config.RemoveEnvMapping(configPath, envName, envVar); err != nil {
			return err
		}
	}
	cacheDeleteSecret(configPath, envName, envVar)
	return nil
}

func findSecretRow(ctx context.Context, configPath, envName, envVar string) (*SecretRow, error) {
	rows, err := ListSecrets(ctx, configPath, envName)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].EnvVar == envVar {
			return &rows[i], nil
		}
	}
	return nil, fmt.Errorf("secret '%s' not found in environment '%s'", envVar, envName)
}
