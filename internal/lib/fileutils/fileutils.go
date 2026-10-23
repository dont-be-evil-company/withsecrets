package fileutils

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/dont-be-evil-company/withsecrets/internal/templates"
	"github.com/spf13/afero"
)

// FileSystem interface for filesystem operations
type FileSystem interface {
	Create(name string) (afero.File, error)
	MkdirAll(path string, perm os.FileMode) error
	OpenFile(name string, flag int, perm os.FileMode) (afero.File, error)
	Stat(name string) (os.FileInfo, error)
	UserConfigDir() (string, error)
	TempDir() string
	Getenv(key string) string
	WriteString(file afero.File, s string) (int, error)
	Close(file afero.File) error
}

// defaultFileSystem implements FileSystem using Afero
type defaultFileSystem struct {
	fs afero.Fs
}

func (d *defaultFileSystem) Create(name string) (afero.File, error) {
	return d.fs.Create(name)
}

func (d *defaultFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return d.fs.MkdirAll(path, perm)
}

func (d *defaultFileSystem) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	return d.fs.OpenFile(name, flag, perm)
}

func (d *defaultFileSystem) Stat(name string) (os.FileInfo, error) {
	return d.fs.Stat(name)
}

func (d *defaultFileSystem) UserConfigDir() (string, error) {
	return os.UserConfigDir()
}

func (d *defaultFileSystem) TempDir() string {
	return os.TempDir()
}

func (d *defaultFileSystem) Getenv(key string) string {
	return os.Getenv(key)
}

func (d *defaultFileSystem) WriteString(file afero.File, s string) (int, error) {
	return file.WriteString(s)
}

func (d *defaultFileSystem) Close(file afero.File) error {
	return file.Close()
}

// Global variables for dependency injection
var (
	fileSystem FileSystem = &defaultFileSystem{fs: afero.NewOsFs()}
)

func JoinPath(elem ...string) string {
	return filepath.Join(elem...)
}

// SetFileSystem sets the file system implementation
func SetFileSystem(fs FileSystem) {
	fileSystem = fs
}

// ResetDependencies resets all dependencies to their default implementations
func ResetDependencies() {
	fileSystem = &defaultFileSystem{fs: afero.NewOsFs()}
}

func FileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err :=
		fileSystem.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil
}

// GenerateDefaultConfig creates a default ws.yaml file in the current directory if it doesn't exist.
func GenerateDefaultConfig() bool {
	fp := config.DefaultConfigFileName

	if FileExists(fp) {
		return false
	}

	b, _, err := templates.ResolveInitTemplate("")
	if err != nil {
		fmt.Println("Error loading default template:", err)
		return false
	}
	contents := string(b)

	file, err := fileSystem.Create(fp)
	if err != nil {
		fmt.Printf("Error creating %s: %v\n", fp, err)
		return false
	}
	defer func() {
		if closeErr := fileSystem.Close(file); closeErr != nil {
			fmt.Printf("Warning: failed to close %s: %v\n", fp, closeErr)
		}
	}()

	_, err = fileSystem.WriteString(file, contents)
	if err != nil {
		fmt.Printf("Error writing to %s: %v\n", fp, err)
		return false
	}

	return true
}

// GetAppDataPath returns the path to the app data directory.
// WS_HOME overrides the default user config directory.
func GetAppDataPath() string {
	if wsHome := fileSystem.Getenv("WS_HOME"); wsHome != "" {
		return EnsureDirExists(wsHome)
	}
	userConfigDir, err := fileSystem.UserConfigDir()
	if err != nil {
		panic(err)
	}
	return EnsureDirExists(userConfigDir + string(os.PathSeparator) + config.AppConfigDirName)
}

// GetTempPath returns the path to the temp directory
// e.g. /tmp
func GetTempPath() string {
	return fileSystem.TempDir()
}

func EnsureDirExists(path string) string {
	if _, err := fileSystem.Stat(path); os.IsNotExist(err) {
		if err := fileSystem.MkdirAll(path, 0755); err != nil {
			// Log the error but don't fail the function
			fmt.Printf("Warning: failed to create directory %s: %v\n", path, err)
		}
	}
	return path
}
