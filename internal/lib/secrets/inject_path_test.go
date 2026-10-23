package secrets

import (
	"testing"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
)

func TestInjectPathSecrets(t *testing.T) {
	t.Run("star injects without prefix", func(t *testing.T) {
		all := make(map[string]string)
		injectPathSecrets(all, nil, "*", "", map[string]string{
			"USERNAME": "alice",
			"PASSWORD": "s3cret",
		})
		if all["USERNAME"] != "alice" || all["PASSWORD"] != "s3cret" {
			t.Fatalf("unexpected map: %#v", all)
		}
	})

	t.Run("named key prefixes", func(t *testing.T) {
		all := make(map[string]string)
		injectPathSecrets(all, nil, "DB", "", map[string]string{
			"USERNAME": "alice",
			"PASSWORD": "s3cret",
		})
		if all["DB_USERNAME"] != "alice" || all["DB_PASSWORD"] != "s3cret" {
			t.Fatalf("unexpected map: %#v", all)
		}
	})

	t.Run("later path overlays earlier for star", func(t *testing.T) {
		all := make(map[string]string)
		refs := make(map[string]string)
		injectPathSecrets(all, refs, "*", "/a", map[string]string{
			"/a/USERNAME": "alice",
			"/a/PASSWORD": "first",
		})
		injectPathSecrets(all, refs, "*", "/b", map[string]string{
			"/b/PASSWORD": "second",
			"/b/TOKEN":    "t",
		})
		if all["USERNAME"] != "alice" || all["PASSWORD"] != "second" || all["TOKEN"] != "t" {
			t.Fatalf("unexpected map: %#v", all)
		}
		if refs["PASSWORD"] != "/b/PASSWORD" || refs["USERNAME"] != "/a/USERNAME" {
			t.Fatalf("unexpected refs: %#v", refs)
		}
	})

	t.Run("later path overlays earlier for named prefix", func(t *testing.T) {
		all := make(map[string]string)
		injectPathSecrets(all, nil, "DB", "/a", map[string]string{
			"/a/USERNAME": "alice",
			"/a/PASSWORD": "first",
		})
		injectPathSecrets(all, nil, "DB", "/b", map[string]string{
			"/b/PASSWORD": "second",
		})
		if all["DB_USERNAME"] != "alice" || all["DB_PASSWORD"] != "second" {
			t.Fatalf("unexpected map: %#v", all)
		}
	})
}

func TestResolvePathEntries(t *testing.T) {
	t.Run("star injects without prefix", func(t *testing.T) {
		entries := resolvePathEntries("*", "", map[string]string{
			"USERNAME": "alice",
			"PASSWORD": "s3cret",
		})
		got := map[string]resolvedPathEntry{}
		for _, e := range entries {
			got[e.EnvironmentVariable] = e
		}
		if got["USERNAME"].Value != "alice" || got["PASSWORD"].Value != "s3cret" {
			t.Fatalf("unexpected entries: %#v", entries)
		}
		if _, ok := got["*"]; ok {
			t.Fatalf("literal * env var generated: %#v", entries)
		}
	})

	t.Run("named key prefixes", func(t *testing.T) {
		entries := resolvePathEntries("DB", "", map[string]string{
			"USERNAME": "alice",
			"PASSWORD": "s3cret",
		})
		got := map[string]string{}
		for _, e := range entries {
			got[e.EnvironmentVariable] = e.Value
		}
		if got["DB_USERNAME"] != "alice" || got["DB_PASSWORD"] != "s3cret" {
			t.Fatalf("unexpected entries: %#v", entries)
		}
	})

	t.Run("sanitizes nested remainder", func(t *testing.T) {
		entries := resolvePathEntries("*", "/app", map[string]string{
			"/app/db/user": "alice",
		})
		if len(entries) != 1 || entries[0].EnvironmentVariable != "DB_USER" {
			t.Fatalf("unexpected entries: %#v", entries)
		}
	})

	t.Run("strips base path prefix", func(t *testing.T) {
		entries := resolvePathEntries("DB", "database", map[string]string{
			"database-username": "alice",
		})
		if len(entries) != 1 || entries[0].EnvironmentVariable != "DB_USERNAME" || entries[0].SourceRef != "database-username" {
			t.Fatalf("unexpected entries: %#v", entries)
		}
	})

	t.Run("deterministic name order", func(t *testing.T) {
		entries := resolvePathEntries("DB", "", map[string]string{
			"ZETA":  "z",
			"ALPHA": "a",
			"MU":    "m",
		})
		if len(entries) != 3 {
			t.Fatalf("unexpected entries: %#v", entries)
		}
		if entries[0].EnvironmentVariable != "DB_ALPHA" || entries[1].EnvironmentVariable != "DB_MU" || entries[2].EnvironmentVariable != "DB_ZETA" {
			t.Fatalf("expected sorted env names, got %#v", entries)
		}
	})

	t.Run("same inputs produce same names", func(t *testing.T) {
		a := resolvePathEntries("DB", "database", map[string]string{"database-username": "alice", "database-password": "s3cret"})
		b := resolvePathEntries("DB", "database", map[string]string{"database-password": "s3cret", "database-username": "alice"})
		if len(a) != len(b) {
			t.Fatalf("length mismatch: %#v vs %#v", a, b)
		}
		for i := range a {
			if a[i].EnvironmentVariable != b[i].EnvironmentVariable || a[i].SourceRef != b[i].SourceRef {
				t.Fatalf("nondeterministic: %#v vs %#v", a, b)
			}
		}
	})
}

func TestPathMappingCacheID(t *testing.T) {
	key := fetchKey{provider: "gcp", project: "example", region: "us-central1"}
	a := pathMappingCacheID(pathMappingKindSecret, key, "DB", []string{"database"})
	b := pathMappingCacheID(pathMappingKindSecret, key, "DB", []string{"database"})
	if a != b || a == "" {
		t.Fatalf("expected stable identity, got %q and %q", a, b)
	}
	param := pathMappingCacheID(pathMappingKindParam, key, "DB", []string{"database"})
	if a == param {
		t.Fatal("secret-path and param-path identities must differ")
	}
	ordered := pathMappingCacheID(pathMappingKindSecret, key, "*", []string{"/shared", "/overrides"})
	reversed := pathMappingCacheID(pathMappingKindSecret, key, "*", []string{"/overrides", "/shared"})
	if ordered == reversed {
		t.Fatal("path order must be part of identity")
	}
}

func TestResolveFetchKey(t *testing.T) {
	env := &config.Environment{
		Provider: "aws",
		Project:  "123",
		Region:   "eu-west-3",
	}

	t.Run("uses environment defaults", func(t *testing.T) {
		key := resolveFetchKey(env, config.EnvItem{})
		if key.provider != "aws" || key.project != "123" || key.region != "eu-west-3" {
			t.Fatalf("unexpected key: %#v", key)
		}
	})

	t.Run("item overrides region", func(t *testing.T) {
		key := resolveFetchKey(env, config.EnvItem{Region: "us-east-1"})
		if key.region != "us-east-1" {
			t.Fatalf("expected us-east-1, got %q", key.region)
		}
	})

	t.Run("aws empty project becomes default", func(t *testing.T) {
		key := resolveFetchKey(&config.Environment{Provider: "aws"}, config.EnvItem{})
		if key.project != "default" {
			t.Fatalf("expected default project, got %q", key.project)
		}
	})
}
