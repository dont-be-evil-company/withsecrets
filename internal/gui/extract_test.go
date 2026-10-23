package gui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dont-be-evil-company/withsecrets/internal/lib/version"
	"github.com/stretchr/testify/require"
)

func TestEnsureAssetsExtracted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	version.VERSION = "test-version"

	webDir, err := ensureAssetsExtracted()
	require.NoError(t, err)
	require.DirExists(t, webDir)

	indexPath := filepath.Join(webDir, "index.html")
	require.FileExists(t, indexPath)

	versionPath := filepath.Join(home, ".cache", "withsecrets", "gui", "version.txt")
	b, err := os.ReadFile(versionPath)
	require.NoError(t, err)
	require.Equal(t, "test-version\n", string(b))

	// Second call should be a no-op when version matches.
	webDir2, err := ensureAssetsExtracted()
	require.NoError(t, err)
	require.Equal(t, webDir, webDir2)
}
