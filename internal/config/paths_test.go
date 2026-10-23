package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindConfigFilePriority(t *testing.T) {
	root := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	write := func(name string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte("default:\n  provider: local\n  env:\n    X:\n      value: y\n"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	if _, err := FindConfigFile(); err == nil {
		t.Fatalf("expected error when no config exists")
	}

	write("withsecrets.yaml")
	got, err := FindConfigFile()
	if err != nil || filepath.Base(got) != "withsecrets.yaml" {
		t.Fatalf("expected withsecrets.yaml, got %q err=%v", got, err)
	}

	write("ws.yaml")
	got, err = FindConfigFile()
	if err != nil || filepath.Base(got) != "ws.yaml" {
		t.Fatalf("expected ws.yaml, got %q err=%v", got, err)
	}
}
