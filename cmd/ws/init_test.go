package ws

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dont-be-evil-company/withsecrets/internal/templates"
)

func TestInitUsesUserDefaultTemplate(t *testing.T) {
	wd := t.TempDir()
	home := t.TempDir()
	t.Setenv("WS_HOME", home)

	dir, err := templates.EnsureTemplatesDir()
	if err != nil {
		t.Fatalf("EnsureTemplatesDir failed: %v", err)
	}
	want := "default:\n  provider: gcp\n  project: from-init-test\n"
	if err := os.WriteFile(filepath.Join(dir, "default.yaml"), []byte(want), 0644); err != nil {
		t.Fatalf("write user default template: %v", err)
	}

	oldWD, _ := os.Getwd()
	if err := os.Chdir(wd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	if err := initCmd.RunE(initCmd, []string{}); err != nil {
		t.Fatalf("init command failed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(wd, "ws.yaml"))
	if err != nil {
		t.Fatalf("read ws.yaml: %v", err)
	}
	if string(got) != want {
		t.Fatalf("ws.yaml mismatch.\nwant:\n%s\ngot:\n%s", want, string(got))
	}
}

func TestInitWithMissingTemplateListsAvailable(t *testing.T) {
	wd := t.TempDir()
	home := t.TempDir()
	t.Setenv("WS_HOME", home)

	dir, err := templates.EnsureTemplatesDir()
	if err != nil {
		t.Fatalf("EnsureTemplatesDir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "alpha.yaml"), []byte("default:\n  provider: gcp\n"), 0644); err != nil {
		t.Fatalf("write template alpha: %v", err)
	}

	oldWD, _ := os.Getwd()
	if err := os.Chdir(wd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	err = initCmd.RunE(initCmd, []string{"missing-template"})
	if err == nil {
		t.Fatalf("expected error for missing template")
	}
	msg := err.Error()
	if !strings.Contains(msg, "does not exist") {
		t.Fatalf("expected missing template error, got: %s", msg)
	}
	if !strings.Contains(msg, "available templates") {
		t.Fatalf("expected available templates list, got: %s", msg)
	}
	if !strings.Contains(msg, "alpha") {
		t.Fatalf("expected template list to contain alpha, got: %s", msg)
	}
}
