package gui

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/version"
)

const versionFileName = "version.txt"

func ensureAssetsExtracted() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve home directory: %w", err)
	}

	guiDir := config.GUIDir(home)
	webDir := config.GUIWebDir(home)
	versionPath := filepath.Join(guiDir, versionFileName)
	currentVersion := strings.TrimSpace(version.VERSION)
	if currentVersion == "" {
		currentVersion = "dev"
	}

	if needsExtract(webDir, versionPath, currentVersion) {
		if err := os.RemoveAll(webDir); err != nil {
			return "", fmt.Errorf("failed to clear gui web cache: %w", err)
		}
		if err := os.MkdirAll(webDir, 0o755); err != nil {
			return "", fmt.Errorf("failed to create gui web cache: %w", err)
		}
		if err := extractEmbeddedFS(webDir); err != nil {
			return "", err
		}
		if err := os.WriteFile(versionPath, []byte(currentVersion+"\n"), 0o644); err != nil {
			return "", fmt.Errorf("failed to write gui version file: %w", err)
		}
	}

	return webDir, nil
}

func needsExtract(webDir, versionPath, currentVersion string) bool {
	if _, err := os.Stat(webDir); err != nil {
		return true
	}
	b, err := os.ReadFile(versionPath)
	if err != nil {
		return true
	}
	return strings.TrimSpace(string(b)) != currentVersion
}

func extractEmbeddedFS(dest string) error {
	return fs.WalkDir(embeddedFS, "dist", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, "dist")
		rel = strings.TrimPrefix(rel, "/")
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		src, err := embeddedFS.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, src)
		return err
	})
}
