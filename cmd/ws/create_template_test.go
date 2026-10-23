package ws

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dont-be-evil-company/withsecrets/internal/templates"
)

func TestOpenInEditorRequiresEditorEnv(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	if err := openInEditor("foo.yaml"); err == nil {
		t.Fatalf("expected error when no VISUAL/EDITOR is set")
	}
}

func TestCreateTemplateCommandCreatesTemplateFile(t *testing.T) {
	t.Setenv("WS_HOME", t.TempDir())
	t.Setenv("EDITOR", "fake-editor")
	t.Setenv("VISUAL", "")

	prevExec := execCommand
	execCommand = func(name string, arg ...string) *exec.Cmd {
		// Return a command that always succeeds to avoid launching a real editor.
		return exec.Command("sh", "-c", "true")
	}
	defer func() { execCommand = prevExec }()

	templateName := "my-template"
	if err := createTemplateCmd.RunE(createTemplateCmd, []string{templateName}); err != nil {
		t.Fatalf("create template command failed: %v", err)
	}

	p, ok, err := templates.ExistingTemplatePath(templateName)
	if err != nil {
		t.Fatalf("ExistingTemplatePath failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected template to exist")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected created template file at %s: %v", p, err)
	}
}

func TestCreateTemplateCommandOpensExistingTemplate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WS_HOME", home)
	t.Setenv("EDITOR", "fake-editor")
	t.Setenv("VISUAL", "")

	dir, err := templates.EnsureTemplatesDir()
	if err != nil {
		t.Fatalf("EnsureTemplatesDir failed: %v", err)
	}
	existing := filepath.Join(dir, "existing.yaml")
	if err := os.WriteFile(existing, []byte("default:\n  provider: gcp\n"), 0644); err != nil {
		t.Fatalf("write existing template: %v", err)
	}

	prevExec := execCommand
	execCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "true")
	}
	defer func() { execCommand = prevExec }()

	if err := createTemplateCmd.RunE(createTemplateCmd, []string{"existing"}); err != nil {
		t.Fatalf("create template command failed: %v", err)
	}
}
