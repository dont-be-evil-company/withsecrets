package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultConfigFileName = "ws.yaml"
	AppConfigDirName      = "withsecrets"
)

// ConfigFileNames lists project config filenames in search priority order.
var ConfigFileNames = []string{"ws.yaml", "withsecrets.yaml"}

// FindConfigFile searches for a project config file in the current directory and parents.
func FindConfigFile() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		for _, name := range ConfigFileNames {
			configPath := filepath.Join(currentDir, name)
			if _, err := os.Stat(configPath); err == nil {
				return configPath, nil
			}
		}

		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			break
		}
		currentDir = parent
	}

	return "", fmt.Errorf("no config file found (%s); searched current directory and parents", stringsJoinConfigNames())
}

func stringsJoinConfigNames() string {
	switch len(ConfigFileNames) {
	case 0:
		return DefaultConfigFileName
	case 1:
		return ConfigFileNames[0]
	default:
		result := ConfigFileNames[0]
		for _, name := range ConfigFileNames[1 : len(ConfigFileNames)-1] {
			result += ", " + name
		}
		return result + ", or " + ConfigFileNames[len(ConfigFileNames)-1]
	}
}

// GlobalConfigDir returns the directory for global withsecrets config (~/.config/withsecrets).
func GlobalConfigDir(homeDir string) string {
	return filepath.Join(homeDir, ".config", AppConfigDirName)
}

// CacheDir returns the withsecrets cache directory (~/.cache/withsecrets).
func CacheDir(homeDir string) string {
	return filepath.Join(homeDir, ".cache", AppConfigDirName)
}

// GUIDir returns the GUI cache directory (~/.cache/withsecrets/gui).
func GUIDir(homeDir string) string {
	return filepath.Join(CacheDir(homeDir), "gui")
}

// GUIWebDir returns the extracted GUI static assets directory.
func GUIWebDir(homeDir string) string {
	return filepath.Join(GUIDir(homeDir), "web")
}
