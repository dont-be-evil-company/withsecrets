package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
		if _, err := os.Stat(tfPath); err != nil {
			return fmt.Errorf("environment '%s': failed to read terraform file %q: %w", name, env.Path, err)
		}

		varFile := strings.TrimSpace(env.VarFile)
		if varFile != "" && !filepath.IsAbs(varFile) {
			varFile = filepath.Join(baseDir, varFile)
		}

		imported, err := terraformEnvFromFile(tfPath, varFile, env.Project)
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
	body, err := parseHCLBody(src, filename)
	if err != nil {
		return nil, err
	}
	items := map[string]EnvItem{}
	if err := importModuleInto(filepath.Dir(filename), []*hclsyntax.Body{body}, nil, envProject, map[string]struct{}{}, items); err != nil {
		return nil, err
	}
	return items, nil
}

// terraformEnvFromFile reads every .tf file next to path, applies varFile when set,
// and imports Cloud Run env from that module and from local modules it calls.
func terraformEnvFromFile(tfPath, varFile, envProject string) (map[string]EnvItem, error) {
	dir := filepath.Dir(tfPath)
	bodies, err := parseTerraformDir(dir)
	if err != nil {
		return nil, err
	}
	var preset map[string]tfValue
	if strings.TrimSpace(varFile) != "" {
		preset, err = loadTFVars(varFile)
		if err != nil {
			return nil, err
		}
	}
	items := map[string]EnvItem{}
	if err := importModuleInto(dir, bodies, preset, envProject, map[string]struct{}{}, items); err != nil {
		return nil, err
	}
	return items, nil
}

func parseHCLBody(src []byte, filename string) (*hclsyntax.Body, error) {
	file, diags := hclsyntax.ParseConfig(src, filename, hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse terraform file: %s", diags.Error())
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("failed to parse terraform file: unexpected body type %T", file.Body)
	}
	return body, nil
}

func parseTerraformDir(dir string) ([]*hclsyntax.Body, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read terraform directory %q: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tf") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	bodies := make([]*hclsyntax.Body, 0, len(names))
	for _, name := range names {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read terraform file %q: %w", path, err)
		}
		body, err := parseHCLBody(data, path)
		if err != nil {
			return nil, err
		}
		bodies = append(bodies, body)
	}
	return bodies, nil
}

// importModuleInto evaluates variables and locals, then collects Cloud Run env.
// preset overrides variable defaults. Local module calls are followed.
// stack holds module directories already being imported, so cycles fail the load.
func importModuleInto(dir string, bodies []*hclsyntax.Body, preset map[string]tfValue, envProject string, stack map[string]struct{}, items map[string]EnvItem) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to read terraform directory %q: %w", dir, err)
	}
	if _, seen := stack[abs]; seen {
		return fmt.Errorf("terraform module cycle at %s", abs)
	}
	stack[abs] = struct{}{}
	defer delete(stack, abs)

	scope := newTFScope()
	for _, body := range bodies {
		for _, block := range body.Blocks {
			if block.Type != "variable" || len(block.Labels) == 0 {
				continue
			}
			if def := attrExpr(block.Body, "default"); def != nil {
				scope.setVar(block.Labels[0], scope.eval(def))
			}
		}
	}
	for name, val := range preset {
		scope.setVar(name, val)
	}
	if scope.vars["project"].isUnset() {
		if project, err := resolveGCPProject("", envProject); err == nil && strings.TrimSpace(project) != "" {
			scope.setVar("project", tfStringVal(project))
		}
	}
	for _, body := range bodies {
		for _, block := range blocksNamed(body, "locals") {
			for name, attr := range block.Body.Attributes {
				scope.localExprs[name] = attr.Expr
			}
		}
	}

	for _, body := range bodies {
		for _, block := range body.Blocks {
			switch block.Type {
			case "resource":
				if len(block.Labels) == 0 {
					continue
				}
				switch block.Labels[0] {
				case tfCloudRunService:
					if err := collectCloudRunEnv(block.Body, scope, envProject, items); err != nil {
						return err
					}
				case tfCloudRunFunction:
					if err := collectCloudFunctionEnv(block.Body, envProject, items); err != nil {
						return err
					}
				}
			case "module":
				if err := followModule(block, abs, scope, envProject, stack, items); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func followModule(block *hclsyntax.Block, parentDir string, scope *tfScope, envProject string, stack map[string]struct{}, items map[string]EnvItem) error {
	source, ok := literalString(attrExpr(block.Body, "source"))
	if !ok || !isLocalModuleSource(source) {
		return nil
	}
	modDir := source
	if !filepath.IsAbs(modDir) {
		modDir = filepath.Join(parentDir, source)
	}
	modDir = filepath.Clean(modDir)
	info, err := os.Stat(modDir)
	if err != nil {
		return fmt.Errorf("failed to read terraform module %q: %w", source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("failed to read terraform module %q: not a directory", source)
	}
	bodies, err := parseTerraformDir(modDir)
	if err != nil {
		return err
	}
	args := map[string]tfValue{}
	for name, attr := range block.Body.Attributes {
		if isModuleMetaArg(name) {
			continue
		}
		args[name] = scope.eval(attr.Expr)
	}
	return importModuleInto(modDir, bodies, args, envProject, stack, items)
}

func isLocalModuleSource(source string) bool {
	source = strings.TrimSpace(source)
	if source == "" {
		return false
	}
	slash := filepath.ToSlash(source)
	if slash == "." || slash == ".." || strings.HasPrefix(slash, "./") || strings.HasPrefix(slash, "../") {
		return true
	}
	return filepath.IsAbs(source)
}

func isModuleMetaArg(name string) bool {
	switch name {
	case "source", "version", "count", "for_each", "providers", "depends_on":
		return true
	default:
		return false
	}
}

func collectCloudRunEnv(body *hclsyntax.Body, scope *tfScope, envProject string, items map[string]EnvItem) error {
	for _, template := range blocksNamed(body, "template") {
		for _, containers := range blocksNamed(template.Body, "containers") {
			if containers.Body == nil {
				continue
			}
			for _, block := range containers.Body.Blocks {
				switch {
				case block.Type == "env":
					if err := addCloudRunEnv(block.Body, envProject, items); err != nil {
						return err
					}
				case block.Type == "dynamic" && len(block.Labels) > 0 && block.Labels[0] == "env":
					if err := addDynamicCloudRunEnv(block, scope, envProject, items); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// addDynamicCloudRunEnv imports a dynamic env block whose for_each is a map.
// name must be the iterator key. value, or secret_key_ref.secret, must be the iterator value.
// The iterator is the explicit name, or Terraform's default "each". The block label is also accepted.
func addDynamicCloudRunEnv(block *hclsyntax.Block, scope *tfScope, envProject string, items map[string]EnvItem) error {
	if scope == nil || block.Body == nil {
		return nil
	}
	each := scope.eval(attrExpr(block.Body, "for_each"))
	if each.kind != tfMap {
		return nil
	}
	contents := blocksNamed(block.Body, "content")
	if len(contents) == 0 || contents[0].Body == nil {
		return nil
	}
	content := contents[0].Body
	iterator, ok := dynamicIterator(block, attrExpr(content, "name"))
	if !ok {
		return nil
	}

	if _, hasValue := content.Attributes["value"]; hasValue {
		if !iteratorAttr(attrExpr(content, "value"), iterator, "value") {
			return nil
		}
		for key, val := range each.mp {
			text, ok := val.asString()
			if !ok || key == "" {
				continue
			}
			items[key] = EnvItem{Value: text}
		}
		return nil
	}

	for _, source := range blocksNamed(content, "value_source") {
		for _, ref := range blocksNamed(source.Body, "secret_key_ref") {
			if !iteratorAttr(attrExpr(ref.Body, "secret"), iterator, "value") {
				continue
			}
			for key, val := range each.mp {
				text, ok := val.asString()
				if !ok || key == "" || text == "" {
					continue
				}
				item, err := gcpSecretItem(key, text, "", envProject)
				if err != nil {
					return err
				}
				items[key] = item
			}
			return nil
		}
	}
	return nil
}

// dynamicIterator returns the iterator name used by name = <iterator>.key.
func dynamicIterator(block *hclsyntax.Block, nameExpr hcl.Expression) (string, bool) {
	candidates := []string{"each"}
	if name, ok := identifierName(attrExpr(block.Body, "iterator")); ok && name != "" {
		candidates = []string{name}
	} else if len(block.Labels) > 0 && block.Labels[0] != "" && block.Labels[0] != "each" {
		candidates = append(candidates, block.Labels[0])
	}
	for _, name := range candidates {
		if iteratorAttr(nameExpr, name, "key") {
			return name, true
		}
	}
	return "", false
}

func iteratorAttr(expr hcl.Expression, iterator, attr string) bool {
	for {
		switch e := expr.(type) {
		case nil:
			return false
		case *hclsyntax.ParenthesesExpr:
			expr = e.Expression
		case *hclsyntax.TemplateWrapExpr:
			expr = e.Wrapped
		case *hclsyntax.ScopeTraversalExpr:
			if len(e.Traversal) != 2 {
				return false
			}
			root, ok := e.Traversal[0].(hcl.TraverseRoot)
			step, okAttr := e.Traversal[1].(hcl.TraverseAttr)
			return ok && okAttr && root.Name == iterator && step.Name == attr
		default:
			return false
		}
	}
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
