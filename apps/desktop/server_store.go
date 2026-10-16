package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/mistweaverco/withsecrets/apps/desktop/app"
)

type savedServer struct {
	URL string `json:"url"`
}

func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "withsecrets", "desktop-settings.json"), nil
}

func loadSavedSettings() (app.DesktopSettings, error) {
	path, err := settingsPath()
	if err != nil {
		return app.DesktopSettings{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return app.DesktopSettings{}, err
	}
	var saved app.DesktopSettings
	if err := json.Unmarshal(raw, &saved); err != nil {
		return app.DesktopSettings{}, err
	}
	return app.NormalizeSettings(saved)
}

func saveSettings(in app.DesktopSettings) error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

func serverPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "withsecrets", "desktop-server.json"), nil
}

func loadSavedServer() (string, error) {
	path, err := serverPath()
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var saved savedServer
	if err := json.Unmarshal(raw, &saved); err != nil {
		return "", err
	}
	return saved.URL, nil
}

func saveServer(rawURL string) error {
	path, err := serverPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	body, err := json.Marshal(savedServer{URL: rawURL})
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}
