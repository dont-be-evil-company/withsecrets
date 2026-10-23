package fileutils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateDefaultConfigUsesUserDefaultTemplate(t *testing.T) {
	wd := t.TempDir()
	home := t.TempDir()
	t.Setenv("WS_HOME", home)

	templatesDir := filepath.Join(home, "templates")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("mkdir templates dir: %v", err)
	}
	expected := "default:\n  provider: gcp\n  project: from-user-template\n"
	if err := os.WriteFile(filepath.Join(templatesDir, "default.yaml"), []byte(expected), 0644); err != nil {
		t.Fatalf("write user default template: %v", err)
	}

	oldWD, _ := os.Getwd()
	if err := os.Chdir(wd); err != nil {
		t.Fatalf("chdir to temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	if ok := GenerateDefaultConfig(); !ok {
		t.Fatalf("GenerateDefaultConfig() returned false")
	}

	got, err := os.ReadFile(filepath.Join(wd, "ws.yaml"))
	if err != nil {
		t.Fatalf("read generated ws.yaml: %v", err)
	}
	if string(got) != expected {
		t.Fatalf("generated ws.yaml mismatch.\nwant:\n%s\ngot:\n%s", expected, string(got))
	}
}

func TestGenerateDefaultConfigFallsBackToEmbeddedTemplate(t *testing.T) {
	wd := t.TempDir()
	t.Setenv("WS_HOME", t.TempDir())

	oldWD, _ := os.Getwd()
	if err := os.Chdir(wd); err != nil {
		t.Fatalf("chdir to temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	if ok := GenerateDefaultConfig(); !ok {
		t.Fatalf("GenerateDefaultConfig() returned false")
	}

	got, err := os.ReadFile(filepath.Join(wd, "ws.yaml"))
	if err != nil {
		t.Fatalf("read generated ws.yaml: %v", err)
	}
	if !strings.Contains(string(got), "insert_provider_here") {
		t.Fatalf("expected embedded template content, got: %s", string(got))
	}
}
