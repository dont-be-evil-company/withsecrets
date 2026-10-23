package templates

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed default.yaml
var embeddedTemplates embed.FS

func DefaultTemplate() ([]byte, error) {
	return embeddedTemplates.ReadFile("default.yaml")
}

func TemplatesDir() string {
	return filepath.Join(appDataPath(), "templates")
}

func EnsureTemplatesDir() (string, error) {
	dir := TemplatesDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create templates directory: %w", err)
	}
	return dir, nil
}

func ValidateTemplateName(name string) error {
	n := strings.TrimSpace(name)
	if n == "" {
		return fmt.Errorf("template name cannot be empty")
	}
	if strings.Contains(n, string(os.PathSeparator)) || strings.Contains(n, "/") || strings.Contains(n, "\\") {
		return fmt.Errorf("template name must not contain path separators")
	}
	return nil
}

func normalizeName(name string) string {
	n := strings.TrimSpace(name)
	n = strings.TrimSuffix(n, ".yaml")
	n = strings.TrimSuffix(n, ".yml")
	return n
}

func TemplatePath(name string) (string, error) {
	if err := ValidateTemplateName(name); err != nil {
		return "", err
	}
	n := normalizeName(name)
	return filepath.Join(TemplatesDir(), n+".yaml"), nil
}

func ExistingTemplatePath(name string) (string, bool, error) {
	if err := ValidateTemplateName(name); err != nil {
		return "", false, err
	}
	n := normalizeName(name)
	candidates := []string{
		filepath.Join(TemplatesDir(), n+".yaml"),
		filepath.Join(TemplatesDir(), n+".yml"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, true, nil
		}
	}
	return candidates[0], false, nil
}

func appDataPath() string {
	if wsHome := os.Getenv("WS_HOME"); wsHome != "" {
		_ = os.MkdirAll(wsHome, 0755)
		return wsHome
	}
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		panic(err)
	}
	path := filepath.Join(userConfigDir, "withsecrets")
	_ = os.MkdirAll(path, 0755)
	return path
}

func ListTemplateNames() ([]string, error) {
	dir, err := EnsureTemplatesDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read templates directory: %w", err)
	}
	seen := map[string]bool{}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ext)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func LoadUserTemplate(name string) ([]byte, string, error) {
	p, ok, err := ExistingTemplatePath(name)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", fmt.Errorf("template '%s' does not exist", normalizeName(name))
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read template '%s': %w", normalizeName(name), err)
	}
	return b, p, nil
}

// ResolveInitTemplate resolves template content for `ws init`.
//
// Rules:
// - If `name` is provided: load that user template.
// - If no name: prefer user template "default", else fallback to embedded default.
func ResolveInitTemplate(name string) ([]byte, string, error) {
	if strings.TrimSpace(name) != "" {
		b, _, err := LoadUserTemplate(name)
		if err != nil {
			return nil, "", err
		}
		return b, normalizeName(name), nil
	}

	if b, _, err := LoadUserTemplate("default"); err == nil {
		return b, "default", nil
	}
	b, err := DefaultTemplate()
	if err != nil {
		return nil, "", fmt.Errorf("failed to load embedded default template: %w", err)
	}
	return b, "embedded-default", nil
}
