package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveInitTemplateFallsBackToEmbeddedDefault(t *testing.T) {
	t.Setenv("WS_HOME", t.TempDir())

	body, source, err := ResolveInitTemplate("")
	if err != nil {
		t.Fatalf("ResolveInitTemplate() returned error: %v", err)
	}
	if source != "embedded-default" {
		t.Fatalf("expected source embedded-default, got %q", source)
	}
	if !strings.Contains(string(body), "insert_provider_here") {
		t.Fatalf("expected embedded template body, got: %s", string(body))
	}
}

func TestResolveInitTemplateUsesUserDefaultTemplateWhenPresent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WS_HOME", home)

	dir, err := EnsureTemplatesDir()
	if err != nil {
		t.Fatalf("EnsureTemplatesDir() error: %v", err)
	}
	userDefault := filepath.Join(dir, "default.yaml")
	content := []byte("default:\n  provider: gcp\n")
	if err := os.WriteFile(userDefault, content, 0644); err != nil {
		t.Fatalf("write user default: %v", err)
	}

	body, source, err := ResolveInitTemplate("")
	if err != nil {
		t.Fatalf("ResolveInitTemplate() returned error: %v", err)
	}
	if source != "default" {
		t.Fatalf("expected source default, got %q", source)
	}
	if string(body) != string(content) {
		t.Fatalf("expected user default body, got: %s", string(body))
	}
}

func TestResolveInitTemplateWithExplicitName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WS_HOME", home)

	dir, err := EnsureTemplatesDir()
	if err != nil {
		t.Fatalf("EnsureTemplatesDir() error: %v", err)
	}
	custom := filepath.Join(dir, "my-template.yaml")
	content := []byte("default:\n  provider: aws\n")
	if err := os.WriteFile(custom, content, 0644); err != nil {
		t.Fatalf("write custom template: %v", err)
	}

	body, source, err := ResolveInitTemplate("my-template")
	if err != nil {
		t.Fatalf("ResolveInitTemplate(my-template) error: %v", err)
	}
	if source != "my-template" {
		t.Fatalf("expected source my-template, got %q", source)
	}
	if string(body) != string(content) {
		t.Fatalf("expected custom template body, got: %s", string(body))
	}
}
