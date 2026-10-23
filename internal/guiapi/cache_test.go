package guiapi

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dont-be-evil-company/withsecrets/internal/lib/cache"
	"github.com/stretchr/testify/require"
)

func setupGuiapiCacheHome(t *testing.T, enabled bool) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	cfgDir := filepath.Join(home, ".config", "withsecrets")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	body := "cache:\n  enabled: false\n"
	if enabled {
		body = "cache:\n  enabled: true\n  ttl: 1h\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(body), 0644))

	configPath := filepath.Join(t.TempDir(), "ws.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`default:
  provider: aws
  project: test
  env:
    SECRET_VAR:
      secret-key: my-secret
`), 0644))
	return configPath
}

func testCacheManager(t *testing.T) *cache.Manager {
	t.Helper()
	mgr, err := cache.NewManager(&cache.GlobalConfig{
		Cache: cache.CacheConfig{Enabled: true, TTL: time.Hour},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })
	return mgr
}

func TestCacheSetSecretWritesDirectKeyAndPathBlob(t *testing.T) {
	configPath := setupGuiapiCacheHome(t, true)
	mgr := testCacheManager(t)

	mapping := cache.CachedPathMapping{
		Entries: []cache.CachedPathEntry{
			{EnvironmentVariable: "SECRET_VAR", SourceRef: "my-secret", Value: "old"},
			{EnvironmentVariable: "OTHER", SourceRef: "other", Value: "keep"},
		},
	}
	require.NoError(t, mgr.SetPathMapping(configPath, "default", "map1", mapping, time.Hour))

	cacheSetSecret(configPath, "default", "SECRET_VAR", "fresh")

	got, found, err := mgr.Get(configPath, "default", "SECRET_VAR")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "fresh", got)

	blob, found, err := mgr.GetPathMapping(configPath, "default", "map1")
	require.NoError(t, err)
	require.True(t, found)
	values := map[string]string{}
	for _, e := range blob.Entries {
		values[e.EnvironmentVariable] = e.Value
	}
	require.Equal(t, "fresh", values["SECRET_VAR"])
	require.Equal(t, "keep", values["OTHER"])
}

func TestCacheDeleteSecretRemovesDirectKeyAndPathEntry(t *testing.T) {
	configPath := setupGuiapiCacheHome(t, true)
	mgr := testCacheManager(t)

	require.NoError(t, mgr.Set(configPath, "default", "SECRET_VAR", "old", time.Hour))
	require.NoError(t, mgr.SetPathMapping(configPath, "default", "map1", cache.CachedPathMapping{
		Entries: []cache.CachedPathEntry{
			{EnvironmentVariable: "SECRET_VAR", SourceRef: "my-secret", Value: "old"},
			{EnvironmentVariable: "OTHER", SourceRef: "other", Value: "keep"},
		},
	}, time.Hour))

	cacheDeleteSecret(configPath, "default", "SECRET_VAR")

	_, found, err := mgr.Get(configPath, "default", "SECRET_VAR")
	require.NoError(t, err)
	require.False(t, found)

	blob, found, err := mgr.GetPathMapping(configPath, "default", "map1")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, blob.Entries, 1)
	require.Equal(t, "OTHER", blob.Entries[0].EnvironmentVariable)
}

func TestCacheClearEnvRemovesAllRows(t *testing.T) {
	configPath := setupGuiapiCacheHome(t, true)
	mgr := testCacheManager(t)

	require.NoError(t, mgr.Set(configPath, "default", "SECRET_VAR", "old", time.Hour))
	require.NoError(t, mgr.SetPathMapping(configPath, "default", "map1", cache.CachedPathMapping{
		Entries: []cache.CachedPathEntry{
			{EnvironmentVariable: "OTHER", SourceRef: "other", Value: "keep"},
		},
	}, time.Hour))

	cacheClearEnv(configPath, "default")

	_, found, err := mgr.Get(configPath, "default", "SECRET_VAR")
	require.NoError(t, err)
	require.False(t, found)
	_, found, err = mgr.GetPathMapping(configPath, "default", "map1")
	require.NoError(t, err)
	require.False(t, found)
}

func TestCacheSyncNoopWhenDisabled(t *testing.T) {
	configPath := setupGuiapiCacheHome(t, false)

	cacheSetSecret(configPath, "default", "SECRET_VAR", "fresh")
	cacheDeleteSecret(configPath, "default", "SECRET_VAR")
	cacheClearEnv(configPath, "default")

	// Opening an enabled manager against the same (never-created) cache dir
	// should still find nothing.
	mgr := testCacheManager(t)
	_, found, err := mgr.Get(configPath, "default", "SECRET_VAR")
	require.NoError(t, err)
	require.False(t, found)
}
