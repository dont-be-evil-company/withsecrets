package ws

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dont-be-evil-company/withsecrets/internal/lib/log"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/version"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update ws to the latest version",
	Long: `Check if a newer version of ws is available and update to it if found.

This command will:
1. Check the current version against the latest GitHub release
2. If a newer version is available, download it
3. Backup the current binary
4. Replace the current binary with the new version

The update process follows the same backup strategy as the installation scripts.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUpdate()
	},
}

var currentGOOS = runtime.GOOS
var newExecCommand = exec.Command

// GitHubRelease represents a GitHub release
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// getCurrentVersion returns the current version of ws
func getCurrentVersion() string {
	return version.VERSION
}

// getLatestVersion fetches the latest release version from GitHub
func getLatestVersion() (string, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Get the latest release
	resp, err := client.Get("https://api.github.com/repos/dont-be-evil-company/withsecrets/releases/latest")
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var release GitHubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("failed to parse release data: %w", err)
	}

	return release.TagName, nil
}

// compareVersions compares two semantic versions
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func compareVersions(v1, v2 string) int {
	// Remove 'v' prefix if present
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	// Split versions into parts
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	// Ensure both versions have the same number of parts
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	// Pad with zeros if necessary
	for len(parts1) < maxLen {
		parts1 = append(parts1, "0")
	}
	for len(parts2) < maxLen {
		parts2 = append(parts2, "0")
	}

	// Compare each part
	for i := 0; i < maxLen; i++ {
		var num1, num2 int
		fmt.Sscanf(parts1[i], "%d", &num1)
		fmt.Sscanf(parts2[i], "%d", &num2)

		if num1 < num2 {
			return -1
		} else if num1 > num2 {
			return 1
		}
	}

	return 0
}

// getCurrentBinaryPath returns the path to the current ws binary
func getCurrentBinaryPath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks to get the actual file path
	resolvedPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		// If symlink resolution fails, use the original path
		resolvedPath = execPath
	}

	return resolvedPath, nil
}

// detectPlatform returns the platform string for the current system
func detectPlatform() string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	// Map Go architecture names to release asset names
	switch arch {
	case "amd64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	case "arm":
		arch = "armv7"
	default:
		arch = "amd64" // fallback
	}

	return fmt.Sprintf("%s-%s", os, arch)
}

// downloadBinary downloads the specified version of ws for the current platform
func downloadBinary(version, platform string) (string, error) {
	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	// Construct download URL
	fileName := fmt.Sprintf("%s-%s", BinaryAssetName(), platform)
	if platform == "windows-amd64" {
		fileName += ".exe"
	}

	downloadURL := fmt.Sprintf("https://github.com/dont-be-evil-company/withsecrets/releases/download/%s/%s", version, fileName)

	// Create temporary file
	tempFile, err := os.CreateTemp("", "withsecrets-update-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer tempFile.Close()

	// Download the binary
	resp, err := client.Get(downloadURL)
	if err != nil {
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to download binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to download binary: HTTP %d", resp.StatusCode)
	}

	// Copy the response to the temporary file
	_, err = io.Copy(tempFile, resp.Body)
	if err != nil {
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to save binary: %w", err)
	}

	// Make the file executable on Unix-like systems
	if platform != "windows-amd64" {
		if err := os.Chmod(tempFile.Name(), 0755); err != nil {
			os.Remove(tempFile.Name())
			return "", fmt.Errorf("failed to make binary executable: %w", err)
		}
	}

	return tempFile.Name(), nil
}

// backupCurrentBinary creates a backup of the current binary
func backupCurrentBinary(binaryPath string) (string, error) {
	timestamp := time.Now().Format("20060102_150405")
	backupPath := fmt.Sprintf("%s.backup.%s", binaryPath, timestamp)

	if err := copyFile(binaryPath, backupPath); err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	return backupPath, nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	// Copy file permissions
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return err
	}

	return os.Chmod(dst, sourceInfo.Mode())
}

// replaceBinary replaces the current binary with the new one
func replaceBinary(currentPath, newBinaryPath string) error {
	if currentGOOS == "windows" {
		return replaceBinaryWindows(currentPath, newBinaryPath)
	}

	// Remove the current binary
	if err := os.Remove(currentPath); err != nil {
		return fmt.Errorf("failed to remove current binary: %w", err)
	}

	// Copy the new binary to the current location
	if err := copyFile(newBinaryPath, currentPath); err != nil {
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	return nil
}

// replaceBinaryWindows schedules a deferred binary replacement in a detached
// PowerShell process so the current executable can exit and release its lock.
func replaceBinaryWindows(currentPath, newBinaryPath string) error {
	stagedPath := currentPath + ".new"
	if err := copyFile(newBinaryPath, stagedPath); err != nil {
		return fmt.Errorf("failed to stage new binary: %w", err)
	}

	scriptFile, err := os.CreateTemp("", "ws-update-*.ps1")
	if err != nil {
		_ = os.Remove(stagedPath)
		return fmt.Errorf("failed to create updater script: %w", err)
	}
	defer scriptFile.Close()

	// Retry copy for up to 60s while current process exits and file lock clears.
	script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$current = '%s'
$staged = '%s'
$maxAttempts = 120

for ($i = 0; $i -lt $maxAttempts; $i++) {
  try {
    Copy-Item -Path $staged -Destination $current -Force
    Remove-Item -Path $staged -Force -ErrorAction SilentlyContinue
    Remove-Item -Path $PSCommandPath -Force -ErrorAction SilentlyContinue
    exit 0
  } catch {
    Start-Sleep -Milliseconds 500
  }
}

exit 1
`, escapePowerShellSingleQuotedPath(currentPath), escapePowerShellSingleQuotedPath(stagedPath))

	if _, err := scriptFile.WriteString(script); err != nil {
		_ = os.Remove(stagedPath)
		_ = os.Remove(scriptFile.Name())
		return fmt.Errorf("failed to write updater script: %w", err)
	}

	cmd := newExecCommand("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptFile.Name())
	if err := cmd.Start(); err != nil {
		_ = os.Remove(stagedPath)
		_ = os.Remove(scriptFile.Name())
		return fmt.Errorf("failed to start updater helper: %w", err)
	}

	return nil
}

func escapePowerShellSingleQuotedPath(path string) string {
	return strings.ReplaceAll(path, "'", "''")
}

// runUpdate executes the update process
func runUpdate() error {
	logger := log.NewLogger()

	// Get current version
	currentVersion := getCurrentVersion()
	logger.Debug("Current version", "version", currentVersion)

	// Get latest version
	logger.Debug("Fetching latest version from GitHub")
	latestVersion, err := getLatestVersion()
	if err != nil {
		return fmt.Errorf("failed to get latest version: %w", err)
	}
	logger.Debug("Latest version", "version", latestVersion)

	// Compare versions
	comparison := compareVersions(currentVersion, latestVersion)
	if comparison >= 0 {
		fmt.Printf("ws is already up to date (version %s)\n", currentVersion)
		return nil
	}

	fmt.Printf("New version available: %s (current: %s)\n", latestVersion, currentVersion)

	// Get current binary path
	currentPath, err := getCurrentBinaryPath()
	if err != nil {
		return fmt.Errorf("failed to get current binary path: %w", err)
	}
	logger.Debug("Current binary path", "path", currentPath)

	// Detect platform
	platform := detectPlatform()
	logger.Debug("Detected platform", "platform", platform)

	// Download new binary
	fmt.Printf("Downloading %s %s for %s...\n", CLIName(), latestVersion, platform)
	newBinaryPath, err := downloadBinary(latestVersion, platform)
	if err != nil {
		return fmt.Errorf("failed to download new version: %w", err)
	}
	defer os.Remove(newBinaryPath) // Clean up temp file

	// Create backup
	fmt.Printf("Creating backup of current binary...\n")
	backupPath, err := backupCurrentBinary(currentPath)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	fmt.Printf("Backup created: %s\n", backupPath)

	// Replace binary
	fmt.Printf("Installing new version...\n")
	if err := replaceBinary(currentPath, newBinaryPath); err != nil {
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	if currentGOOS == "windows" {
		fmt.Printf("Update scheduled from %s to %s\n", currentVersion, latestVersion)
		fmt.Printf("Please wait a few seconds, then run: %s version\n", CLIName())
	} else {
		fmt.Printf("Successfully updated ws from %s to %s\n", currentVersion, latestVersion)
	}
	fmt.Printf("Backup saved as: %s\n", backupPath)

	return nil
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
