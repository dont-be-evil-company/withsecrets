package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

const (
	tfCloudRunService    = "google_cloud_run_v2_service"
	tfCloudRunFunction   = "google_cloudfunctions2_function"
	tfSecretResourceMark = "/secrets/"
)

// expandTerraformEnvironments reads each terraform environment's file and adds
// env items for literal values and secret refs. Keys already set in ws.yaml are left alone.
func expandTerraformEnvironments(config *SecretsConfig, configPath string) error {
	baseDir := filepath.Dir(configPath)
	for name, env := range config.Environments {
		if env.Provider != "terraform" {
			continue
		}
		if strings.TrimSpace(env.Path) == "" {
			return fmt.Errorf("environment '%s': path is required for provider 'terraform'", name)
		}

		tfPath := env.Path
		if !filepath.IsAbs(tfPath) {
			tfPath = filepath.Join(baseDir, tfPath)
		}
		data, err := os.ReadFile(tfPath)
		if err != nil {
			return fmt.Errorf("environment '%s': failed to read terraform file %q: %w", name, env.Path, err)
		}

		imported, err := terraformEnvItems(data, tfPath, env.Project)
		if err != nil {
			return fmt.Errorf("environment '%s': %w", name, err)
		}
		if env.Env == nil {
			env.Env = map[string]EnvItem{}
		}
		for key, item := range imported {
			if _, exists := env.Env[key]; !exists {
				env.Env[key] = item
			}
		}
		config.Environments[name] = env
	}
	return nil
}

// terraformEnvItems returns env items from Cloud Run and Cloud Run function resources.
// Later resources and env blocks replace earlier ones with the same name.
func terraformEnvItems(src []byte, filename, envProject string) (map[string]EnvItem, error) {
	file, diags := hclsyntax.ParseConfig(src, filename, hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse terraform file: %s", diags.Error())
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("failed to parse terraform file: unexpected body type %T", file.Body)
	}

	items := map[string]EnvItem{}
	for _, block := range body.Blocks {
		if block.Type != "resource" || len(block.Labels) == 0 {
			continue
		}
		switch block.Labels[0] {
		case tfCloudRunService:
			if err := collectCloudRunEnv(block.Body, envProject, items); err != nil {
				return nil, err
			}
		case tfCloudRunFunction:
			if err := collectCloudFunctionEnv(block.Body, envProject, items); err != nil {
				return nil, err
			}
		}
	}
	return items, nil
}

func collectCloudRunEnv(body *hclsyntax.Body, envProject string, items map[string]EnvItem) error {
	for _, template := range blocksNamed(body, "template") {
		for _, containers := range blocksNamed(template.Body, "containers") {
			for _, envBlock := range blocksNamed(containers.Body, "env") {
				if err := addCloudRunEnv(envBlock.Body, envProject, items); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func addCloudRunEnv(body *hclsyntax.Body, envProject string, items map[string]EnvItem) error {
	name, ok := literalString(attrExpr(body, "name"))
	if !ok || name == "" {
		return nil
	}
	if _, hasValue := body.Attributes["value"]; hasValue {
		value, ok := literalString(attrExpr(body, "value"))
		if !ok {
			return nil
		}
		items[name] = EnvItem{Value: value}
		return nil
	}

	for _, source := range blocksNamed(body, "value_source") {
		for _, ref := range blocksNamed(source.Body, "secret_key_ref") {
			secretLit, ok := literalString(attrExpr(ref.Body, "secret"))
			if !ok || secretLit == "" {
				return nil
			}
			item, err := gcpSecretItem(name, secretLit, "", envProject)
			if err != nil {
				return err
			}
			items[name] = item
			return nil
		}
	}
	return nil
}

func collectCloudFunctionEnv(body *hclsyntax.Body, envProject string, items map[string]EnvItem) error {
	for _, serviceConfig := range blocksNamed(body, "service_config") {
		for key, value := range objectLiteralStrings(attrExpr(serviceConfig.Body, "environment_variables")) {
			items[key] = EnvItem{Value: value}
		}
		for _, secretBlock := range blocksNamed(serviceConfig.Body, "secret_environment_variables") {
			key, ok := literalString(attrExpr(secretBlock.Body, "key"))
			if !ok || key == "" {
				continue
			}
			secretLit, ok := literalString(attrExpr(secretBlock.Body, "secret"))
			if !ok || secretLit == "" {
				continue
			}
			projectID, _ := literalString(attrExpr(secretBlock.Body, "project_id"))
			item, err := gcpSecretItem(key, secretLit, projectID, envProject)
			if err != nil {
				return err
			}
			items[key] = item
		}
	}
	return nil
}

func gcpSecretItem(envName, secretLit, projectID, envProject string) (EnvItem, error) {
	project, secretID := splitSecretResource(secretLit)
	if project == "" {
		project = strings.TrimSpace(projectID)
	}
	resolved, err := resolveGCPProject(project, envProject)
	if err != nil {
		return EnvItem{}, fmt.Errorf("env %s: %w", envName, err)
	}
	return EnvItem{
		SecretKey: secretID,
		Provider:  "gcp",
		Project:   resolved,
	}, nil
}

// splitSecretResource splits projects/{project}/secrets/{name} into project and name.
// A bare secret id is returned unchanged, with an empty project.
func splitSecretResource(secret string) (project, id string) {
	rest, ok := strings.CutPrefix(secret, "projects/")
	if !ok {
		return "", secret
	}
	project, rest, ok = strings.Cut(rest, tfSecretResourceMark)
	if !ok || project == "" || rest == "" {
		return "", secret
	}
	id, _, _ = strings.Cut(rest, "/")
	if id == "" {
		return "", secret
	}
	return project, id
}

// resolveGCPProject picks a GCP project for a secret ref.
// explicit is a project taken from the resource. envProject is the ws.yaml project.
func resolveGCPProject(explicit, envProject string) (string, error) {
	if project := strings.TrimSpace(explicit); project != "" {
		return project, nil
	}
	if project := strings.TrimSpace(envProject); project != "" {
		return project, nil
	}
	if project := firstEnv("GOOGLE_CLOUD_PROJECT", "GCLOUD_PROJECT", "CLOUDSDK_CORE_PROJECT"); project != "" {
		return project, nil
	}
	if project := gcloudConfigProject(); project != "" {
		return project, nil
	}
	return "", fmt.Errorf("GCP project is required to read the secret: set project, GOOGLE_CLOUD_PROJECT, or gcloud's core.project")
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

// gcloudConfigProject reads core.project from the active gcloud configuration.
func gcloudConfigProject() string {
	configDir := strings.TrimSpace(os.Getenv("CLOUDSDK_CONFIG"))
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		configDir = filepath.Join(home, ".config", "gcloud")
	}

	active := strings.TrimSpace(os.Getenv("CLOUDSDK_ACTIVE_CONFIG_NAME"))
	if active == "" {
		data, err := os.ReadFile(filepath.Join(configDir, "active_config"))
		if err == nil {
			active = strings.TrimSpace(string(data))
		}
	}
	if active == "" {
		active = "default"
	}

	file, err := os.Open(filepath.Join(configDir, "configurations", "config_"+active))
	if err != nil {
		return ""
	}
	defer file.Close()

	inCore := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inCore = strings.EqualFold(line, "[core]")
			continue
		}
		if !inCore {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "project" {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return ""
}

func blocksNamed(body *hclsyntax.Body, name string) []*hclsyntax.Block {
	if body == nil {
		return nil
	}
	var found []*hclsyntax.Block
	for _, block := range body.Blocks {
		if block.Type == name {
			found = append(found, block)
		}
	}
	return found
}

func attrExpr(body *hclsyntax.Body, name string) hcl.Expression {
	if body == nil {
		return nil
	}
	attr, ok := body.Attributes[name]
	if !ok || attr == nil {
		return nil
	}
	return attr.Expr
}

// literalString returns a static HCL string. Variables, interpolations, and ternaries are rejected.
func literalString(expr hcl.Expression) (string, bool) {
	if expr == nil {
		return "", false
	}
	switch e := expr.(type) {
	case *hclsyntax.TemplateExpr:
		if len(e.Parts) != 1 {
			return "", false
		}
		return literalString(e.Parts[0])
	case *hclsyntax.LiteralValueExpr:
		if e.Val.IsNull() || !e.Val.IsKnown() || !e.Val.Type().Equals(cty.String) {
			return "", false
		}
		return e.Val.AsString(), true
	case *hclsyntax.ObjectConsKeyExpr:
		if name, ok := identifierName(e.Wrapped); ok {
			return name, true
		}
		return literalString(e.Wrapped)
	default:
		return "", false
	}
}

func objectLiteralStrings(expr hcl.Expression) map[string]string {
	obj, ok := expr.(*hclsyntax.ObjectConsExpr)
	if !ok || obj == nil {
		return nil
	}
	out := make(map[string]string, len(obj.Items))
	for _, item := range obj.Items {
		key, ok := literalString(item.KeyExpr)
		if !ok || key == "" {
			continue
		}
		value, ok := literalString(item.ValueExpr)
		if !ok {
			continue
		}
		out[key] = value
	}
	return out
}

func identifierName(expr hcl.Expression) (string, bool) {
	traversal, ok := expr.(*hclsyntax.ScopeTraversalExpr)
	if !ok || len(traversal.Traversal) != 1 {
		return "", false
	}
	root, ok := traversal.Traversal[0].(hcl.TraverseRoot)
	if !ok || root.Name == "" {
		return "", false
	}
	return root.Name, true
}
