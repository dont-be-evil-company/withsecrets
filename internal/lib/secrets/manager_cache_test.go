package secrets

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/cache"
	"github.com/stretchr/testify/require"
)

type fakeProvider struct {
	mu                   sync.Mutex
	secrets              map[string]string
	secretsByPath        map[string]map[string]string
	params               map[string]string
	paramsByPath         map[string]map[string]string
	getSecretsN          int
	getSecretsByPathN    int
	getParametersN       int
	getParametersByPathN int
}

func (f *fakeProvider) snapshot() (secrets, secretsByPath, params, paramsByPath int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getSecretsN, f.getSecretsByPathN, f.getParametersN, f.getParametersByPathN
}

func (f *fakeProvider) GetSecret(_, secretID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.secrets[secretID]; ok {
		return v, nil
	}
	return "", fmt.Errorf("secret %q not found", secretID)
}

func (f *fakeProvider) GetSecrets(_ string, secretIDs []string) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getSecretsN++
	out := make(map[string]string)
	for _, id := range secretIDs {
		if v, ok := f.secrets[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

func (f *fakeProvider) GetSecretsByPath(_, secretPath string) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getSecretsByPathN++
	return copyStringMap(f.secretsByPath[secretPath]), nil
}

func (f *fakeProvider) GetParameter(_, paramName string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.params[paramName]; ok {
		return v, nil
	}
	return "", fmt.Errorf("parameter %q not found", paramName)
}

func (f *fakeProvider) GetParameters(_ string, paramNames []string) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getParametersN++
	out := make(map[string]string)
	for _, id := range paramNames {
		if v, ok := f.params[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

func (f *fakeProvider) GetParametersByPath(_, paramPath string) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getParametersByPathN++
	return copyStringMap(f.paramsByPath[paramPath]), nil
}

func (f *fakeProvider) PutParameter(_, _, _ string) error { return nil }
func (f *fakeProvider) DeleteParameter(_, _ string) error { return nil }
func (f *fakeProvider) Close() error                      { return nil }

func copyStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func factoryWithFake(fake *fakeProvider) *SecretManagerFactory {
	return &SecretManagerFactory{
		createManager: func(context.Context, string, string, string) (SecretManager, error) {
			return fake, nil
		},
	}
}

func setupCacheHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	cfgDir := filepath.Join(home, ".config", "withsecrets")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("cache:\n  enabled: true\n  ttl: 1h\n"), 0644))
	return filepath.Join(t.TempDir(), "ws.yaml")
}

func awsEnv(items map[string]config.EnvItem) *config.Environment {
	return &config.Environment{
		Provider: "aws",
		Env:      items,
	}
}

func expireAllCache(t *testing.T) {
	t.Helper()
	c, err := cache.NewCache()
	require.NoError(t, err)
	defer func() { _ = c.Close() }()
	entries, err := c.List()
	require.NoError(t, err)
	for _, e := range entries {
		require.NoError(t, c.Set(e.Path, e.ConfigEnv, e.Env, e.Value, -time.Hour))
	}
}

func corruptPathMappings(t *testing.T) {
	t.Helper()
	c, err := cache.NewCache()
	require.NoError(t, err)
	defer func() { _ = c.Close() }()
	entries, err := c.List()
	require.NoError(t, err)
	n := 0
	for _, e := range entries {
		if cache.IsPathMappingKey(e.Env) {
			require.NoError(t, c.Set(e.Path, e.ConfigEnv, e.Env, "not-json", time.Hour))
			n++
		}
	}
	require.NotZero(t, n, "expected at least one path mapping cache row to corrupt")
}

func lookupTwice(t *testing.T, factory *SecretManagerFactory, env *config.Environment, configPath string) (freshSecrets, freshRefs, cachedSecrets, cachedRefs map[string]string) {
	t.Helper()
	ctx := context.Background()
	var err error
	freshSecrets, freshRefs, err = factory.GetSecretsForEnvironmentWithCache(ctx, env, configPath, "default")
	require.NoError(t, err)
	cachedSecrets, cachedRefs, err = factory.GetSecretsForEnvironmentWithCache(ctx, env, configPath, "default")
	require.NoError(t, err)
	require.Equal(t, freshSecrets, cachedSecrets)
	require.Equal(t, freshRefs, cachedRefs)
	return
}

func TestCacheDirectSecretKey(t *testing.T) {
	configPath := setupCacheHome(t)
	fake := &fakeProvider{secrets: map[string]string{"database-url": "postgres://db"}}
	factory := factoryWithFake(fake)
	env := awsEnv(map[string]config.EnvItem{
		"DATABASE_URL": {SecretKey: "database-url"},
	})

	fresh, freshRefs, cached, cachedRefs := lookupTwice(t, factory, env, configPath)
	require.Equal(t, "postgres://db", fresh["DATABASE_URL"])
	require.Equal(t, "database-url", freshRefs["DATABASE_URL"])
	require.Equal(t, fresh, cached)
	require.Equal(t, freshRefs, cachedRefs)

	s, p, pm, pp := fake.snapshot()
	require.Equal(t, 1, s)
	require.Equal(t, 0, p)
	require.Equal(t, 0, pm)
	require.Equal(t, 0, pp)
}

func TestCacheDirectParamKey(t *testing.T) {
	configPath := setupCacheHome(t)
	fake := &fakeProvider{params: map[string]string{"/application/database-url": "postgres://db"}}
	factory := factoryWithFake(fake)
	env := awsEnv(map[string]config.EnvItem{
		"DATABASE_URL": {ParamKey: "/application/database-url"},
	})

	fresh, freshRefs, _, _ := lookupTwice(t, factory, env, configPath)
	require.Equal(t, "postgres://db", fresh["DATABASE_URL"])
	require.Equal(t, "/application/database-url", freshRefs["DATABASE_URL"])

	s, p, pm, pp := fake.snapshot()
	require.Equal(t, 0, s)
	require.Equal(t, 0, p)
	require.Equal(t, 1, pm)
	require.Equal(t, 0, pp)
}

func TestCachePrefixedSecretPath(t *testing.T) {
	configPath := setupCacheHome(t)
	fake := &fakeProvider{secretsByPath: map[string]map[string]string{
		"database": {
			"database-username": "alice",
			"database-password": "s3cret",
		},
	}}
	factory := factoryWithFake(fake)
	env := awsEnv(map[string]config.EnvItem{
		"DB": {SecretPath: []string{"database"}},
	})

	fresh, freshRefs, _, _ := lookupTwice(t, factory, env, configPath)
	require.Equal(t, "alice", fresh["DB_USERNAME"])
	require.Equal(t, "s3cret", fresh["DB_PASSWORD"])
	require.Equal(t, "database-username", freshRefs["DB_USERNAME"])
	require.Equal(t, "database-password", freshRefs["DB_PASSWORD"])
	require.NotContains(t, fresh, "DB")
	require.NotContains(t, fresh, "*")

	_, p, _, _ := fake.snapshot()
	require.Equal(t, 1, p)
}

func TestCacheWildcardSecretPath(t *testing.T) {
	configPath := setupCacheHome(t)
	fake := &fakeProvider{secretsByPath: map[string]map[string]string{
		"/shared/config": {
			"/shared/config/USERNAME": "alice",
			"/shared/config/PASSWORD": "s3cret",
		},
	}}
	factory := factoryWithFake(fake)
	env := awsEnv(map[string]config.EnvItem{
		"*": {SecretPath: []string{"/shared/config"}},
	})

	fresh, freshRefs, _, _ := lookupTwice(t, factory, env, configPath)
	require.Equal(t, "alice", fresh["USERNAME"])
	require.Equal(t, "s3cret", fresh["PASSWORD"])
	require.Equal(t, "/shared/config/USERNAME", freshRefs["USERNAME"])
	require.NotContains(t, fresh, "*")

	_, p, _, _ := fake.snapshot()
	require.Equal(t, 1, p)
}

func TestCachePrefixedParamPath(t *testing.T) {
	configPath := setupCacheHome(t)
	fake := &fakeProvider{paramsByPath: map[string]map[string]string{
		"/db": {
			"/db/USERNAME": "alice",
			"/db/PASSWORD": "s3cret",
		},
	}}
	factory := factoryWithFake(fake)
	env := awsEnv(map[string]config.EnvItem{
		"DB": {ParamPath: []string{"/db"}},
	})

	fresh, freshRefs, _, _ := lookupTwice(t, factory, env, configPath)
	require.Equal(t, "alice", fresh["DB_USERNAME"])
	require.Equal(t, "s3cret", fresh["DB_PASSWORD"])
	require.Equal(t, "/db/USERNAME", freshRefs["DB_USERNAME"])
	require.NotContains(t, fresh, "DB")

	_, _, _, pp := fake.snapshot()
	require.Equal(t, 1, pp)
}

func TestCacheWildcardParamPath(t *testing.T) {
	configPath := setupCacheHome(t)
	fake := &fakeProvider{paramsByPath: map[string]map[string]string{
		"/shared/config": {
			"/shared/config/USERNAME": "alice",
			"/shared/config/PASSWORD": "s3cret",
		},
	}}
	factory := factoryWithFake(fake)
	env := awsEnv(map[string]config.EnvItem{
		"*": {ParamPath: []string{"/shared/config"}},
	})

	fresh, _, _, _ := lookupTwice(t, factory, env, configPath)
	require.Equal(t, "alice", fresh["USERNAME"])
	require.Equal(t, "s3cret", fresh["PASSWORD"])
	require.NotContains(t, fresh, "*")

	_, _, _, pp := fake.snapshot()
	require.Equal(t, 1, pp)
}

func TestCacheSecretPathOverlay(t *testing.T) {
	configPath := setupCacheHome(t)
	fake := &fakeProvider{secretsByPath: map[string]map[string]string{
		"/shared/config": {
			"/shared/config/USERNAME": "alice",
			"/shared/config/API_URL":  "https://old.example",
		},
		"/overrides/config": {
			"/overrides/config/API_URL": "https://new.example",
		},
	}}
	factory := factoryWithFake(fake)
	env := awsEnv(map[string]config.EnvItem{
		"*": {SecretPath: []string{"/shared/config", "/overrides/config"}},
	})

	fresh, freshRefs, _, _ := lookupTwice(t, factory, env, configPath)
	require.Equal(t, "alice", fresh["USERNAME"])
	require.Equal(t, "https://new.example", fresh["API_URL"])
	require.Equal(t, "/shared/config/USERNAME", freshRefs["USERNAME"])
	require.Equal(t, "/overrides/config/API_URL", freshRefs["API_URL"])

	_, p, _, _ := fake.snapshot()
	require.Equal(t, 2, p)
}

func TestCacheParamPathOverlay(t *testing.T) {
	configPath := setupCacheHome(t)
	fake := &fakeProvider{paramsByPath: map[string]map[string]string{
		"/shared/config": {
			"/shared/config/USERNAME": "alice",
			"/shared/config/API_URL":  "https://old.example",
		},
		"/overrides/config": {
			"/overrides/config/API_URL": "https://new.example",
		},
	}}
	factory := factoryWithFake(fake)
	env := awsEnv(map[string]config.EnvItem{
		"*": {ParamPath: []string{"/shared/config", "/overrides/config"}},
	})

	fresh, freshRefs, _, _ := lookupTwice(t, factory, env, configPath)
	require.Equal(t, "alice", fresh["USERNAME"])
	require.Equal(t, "https://new.example", fresh["API_URL"])
	require.Equal(t, "/overrides/config/API_URL", freshRefs["API_URL"])

	_, _, _, pp := fake.snapshot()
	require.Equal(t, 2, pp)
}

func TestCacheMixedMappings(t *testing.T) {
	configPath := setupCacheHome(t)
	fake := &fakeProvider{
		secrets: map[string]string{"app-token": "t0ken"},
		params:  map[string]string{"/app/region": "eu-west-1"},
		secretsByPath: map[string]map[string]string{
			"database": {
				"database-username": "alice",
				"database-password": "s3cret",
			},
		},
		paramsByPath: map[string]map[string]string{
			"/flags": {"/flags/FEATURE": "on"},
		},
	}
	factory := factoryWithFake(fake)
	env := awsEnv(map[string]config.EnvItem{
		"APP_TOKEN": {SecretKey: "app-token"},
		"REGION":    {ParamKey: "/app/region"},
		"DB":        {SecretPath: []string{"database"}},
		"*":         {ParamPath: []string{"/flags"}},
		"STATIC":    {Value: "hello"},
	})

	fresh, freshRefs, _, _ := lookupTwice(t, factory, env, configPath)
	require.Equal(t, "t0ken", fresh["APP_TOKEN"])
	require.Equal(t, "eu-west-1", fresh["REGION"])
	require.Equal(t, "alice", fresh["DB_USERNAME"])
	require.Equal(t, "on", fresh["FEATURE"])
	require.Equal(t, "hello", fresh["STATIC"])
	require.Equal(t, "app-token", freshRefs["APP_TOKEN"])
	require.Equal(t, "/app/region", freshRefs["REGION"])
	require.Equal(t, "database-username", freshRefs["DB_USERNAME"])
	require.Equal(t, "/flags/FEATURE", freshRefs["FEATURE"])
	require.NotContains(t, freshRefs, "STATIC")

	s, p, pm, pp := fake.snapshot()
	require.Equal(t, 1, s)
	require.Equal(t, 1, p)
	require.Equal(t, 1, pm)
	require.Equal(t, 1, pp)
}

func TestCacheInterpolationWithPathVars(t *testing.T) {
	configPath := setupCacheHome(t)
	fake := &fakeProvider{secretsByPath: map[string]map[string]string{
		"database": {
			"database-username": "alice",
			"database-password": "s3cret",
		},
	}}
	factory := factoryWithFake(fake)
	env := awsEnv(map[string]config.EnvItem{
		"DB":           {SecretPath: []string{"database"}},
		"DATABASE_URL": {Value: "postgres://${DB_USERNAME}:${DB_PASSWORD}@localhost/app"},
	})

	fresh, _, cached, _ := lookupTwice(t, factory, env, configPath)
	require.Equal(t, "postgres://alice:s3cret@localhost/app", fresh["DATABASE_URL"])
	require.Equal(t, fresh["DATABASE_URL"], cached["DATABASE_URL"])

	_, p, _, _ := fake.snapshot()
	require.Equal(t, 1, p)
}

func TestCacheExpirationRefetchesAndReplacesMembers(t *testing.T) {
	configPath := setupCacheHome(t)
	fake := &fakeProvider{secretsByPath: map[string]map[string]string{
		"database": {
			"database-username": "alice",
			"database-password": "s3cret",
		},
	}}
	factory := factoryWithFake(fake)
	env := awsEnv(map[string]config.EnvItem{
		"DB": {SecretPath: []string{"database"}},
	})
	ctx := context.Background()

	fresh, _, err := factory.GetSecretsForEnvironmentWithCache(ctx, env, configPath, "default")
	require.NoError(t, err)
	require.Equal(t, "s3cret", fresh["DB_PASSWORD"])
	require.NotContains(t, fresh, "DB_HOST")

	expireAllCache(t)

	fake.mu.Lock()
	fake.secretsByPath["database"] = map[string]string{
		"database-username": "alice",
		"database-host":     "localhost",
	}
	fake.mu.Unlock()

	refreshed, refreshedRefs, err := factory.GetSecretsForEnvironmentWithCache(ctx, env, configPath, "default")
	require.NoError(t, err)
	require.Equal(t, "alice", refreshed["DB_USERNAME"])
	require.Equal(t, "localhost", refreshed["DB_HOST"])
	require.NotContains(t, refreshed, "DB_PASSWORD")
	require.Equal(t, "database-host", refreshedRefs["DB_HOST"])

	cached, cachedRefs, err := factory.GetSecretsForEnvironmentWithCache(ctx, env, configPath, "default")
	require.NoError(t, err)
	require.Equal(t, refreshed, cached)
	require.Equal(t, refreshedRefs, cachedRefs)

	_, p, _, _ := fake.snapshot()
	require.Equal(t, 2, p)
}

func TestCacheMalformedPathRecordIsMiss(t *testing.T) {
	configPath := setupCacheHome(t)
	fake := &fakeProvider{secretsByPath: map[string]map[string]string{
		"database": {
			"database-username": "alice",
			"database-password": "s3cret",
		},
	}}
	factory := factoryWithFake(fake)
	env := awsEnv(map[string]config.EnvItem{
		"DB": {SecretPath: []string{"database"}},
	})
	ctx := context.Background()

	_, _, err := factory.GetSecretsForEnvironmentWithCache(ctx, env, configPath, "default")
	require.NoError(t, err)

	corruptPathMappings(t)

	_, _, err = factory.GetSecretsForEnvironmentWithCache(ctx, env, configPath, "default")
	require.NoError(t, err)

	_, p, _, _ := fake.snapshot()
	require.Equal(t, 2, p)
}
