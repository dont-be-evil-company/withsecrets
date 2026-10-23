package templates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTemplateNameValidationAndExistingPath(t *testing.T) {
	t.Setenv("WS_HOME", t.TempDir())

	if err := ValidateTemplateName("foo/bar"); err == nil {
		t.Fatalf("expected invalid template name with slash")
	}
	if err := ValidateTemplateName(""); err == nil {
		t.Fatalf("expected invalid template name for empty string")
	}
	if err := ValidateTemplateName("valid-name"); err != nil {
		t.Fatalf("expected valid template name, got: %v", err)
	}

	dir, err := EnsureTemplatesDir()
	if err != nil {
		t.Fatalf("EnsureTemplatesDir failed: %v", err)
	}
	ymlPath := filepath.Join(dir, "alpha.yml")
	if err := os.WriteFile(ymlPath, []byte("default:\n  provider: gcp\n"), 0644); err != nil {
		t.Fatalf("write template yml: %v", err)
	}

	p, ok, err := ExistingTemplatePath("alpha")
	if err != nil {
		t.Fatalf("ExistingTemplatePath error: %v", err)
	}
	if !ok {
		t.Fatalf("expected template alpha to exist")
	}
	if p != ymlPath {
		t.Fatalf("expected path %s, got %s", ymlPath, p)
	}
}

func TestListTemplateNames(t *testing.T) {
	t.Setenv("WS_HOME", t.TempDir())
	dir, err := EnsureTemplatesDir()
	if err != nil {
		t.Fatalf("EnsureTemplatesDir failed: %v", err)
	}
	_ = os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(""), 0644)
	_ = os.WriteFile(filepath.Join(dir, "b.yml"), []byte(""), 0644)
	_ = os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte(""), 0644)

	names, err := ListTemplateNames()
	if err != nil {
		t.Fatalf("ListTemplateNames failed: %v", err)
	}
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Fatalf("unexpected names: %#v", names)
	}
}
