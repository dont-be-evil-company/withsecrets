package cache

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestInitSchemaUsesConfigEnvColumn(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.sqlite")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cache := &Cache{db: db}
	if err := cache.initSchema(); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	if !tableHasColumn(t, db, "secrets", "config_env") {
		t.Fatalf("expected secrets.config_env column")
	}
	if tableHasColumn(t, db, "secrets", "kuba_env") {
		t.Fatalf("did not expect secrets.kuba_env column")
	}
}

func tableHasColumn(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid       int
			name      string
			colType   string
			notNull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info: %v", err)
	}
	return false
}

func TestCacheDeleteRemovesRow(t *testing.T) {
	c := newTestCache(t)
	if err := c.Set("/tmp/ws.yaml", "default", "FOO", "old", time.Hour); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := c.Delete("/tmp/ws.yaml", "default", "FOO"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, found, err := c.Get("/tmp/ws.yaml", "default", "FOO")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found {
		t.Fatal("expected FOO to be deleted")
	}
}

func TestPatchPathMappingEntryUpdatesValue(t *testing.T) {
	c := newTestCache(t)
	mapping := CachedPathMapping{
		Entries: []CachedPathEntry{
			{EnvironmentVariable: "DB_URL", SourceRef: "db-url", Value: "old"},
			{EnvironmentVariable: "DB_PASS", SourceRef: "db-pass", Value: "secret"},
		},
	}
	if err := c.SetPathMapping("/tmp/ws.yaml", "default", "map1", mapping, time.Hour); err != nil {
		t.Fatalf("SetPathMapping: %v", err)
	}
	if err := c.PatchPathMappingEntry("/tmp/ws.yaml", "default", "DB_URL", "new"); err != nil {
		t.Fatalf("PatchPathMappingEntry: %v", err)
	}

	got, found, err := c.GetPathMapping("/tmp/ws.yaml", "default", "map1")
	if err != nil {
		t.Fatalf("GetPathMapping: %v", err)
	}
	if !found {
		t.Fatal("expected path mapping to remain")
	}
	if got.Entries[0].EnvironmentVariable != "DB_PASS" && got.Entries[1].EnvironmentVariable != "DB_PASS" {
		t.Fatalf("unexpected entries: %+v", got.Entries)
	}
	values := map[string]string{}
	for _, e := range got.Entries {
		values[e.EnvironmentVariable] = e.Value
	}
	if values["DB_URL"] != "new" {
		t.Fatalf("DB_URL = %q, want new", values["DB_URL"])
	}
	if values["DB_PASS"] != "secret" {
		t.Fatalf("DB_PASS = %q, want secret", values["DB_PASS"])
	}
}

func TestRemovePathMappingEntryDropsKeyAndEmptyBlob(t *testing.T) {
	c := newTestCache(t)
	mapping := CachedPathMapping{
		Entries: []CachedPathEntry{
			{EnvironmentVariable: "DB_URL", SourceRef: "db-url", Value: "old"},
		},
	}
	if err := c.SetPathMapping("/tmp/ws.yaml", "default", "map1", mapping, time.Hour); err != nil {
		t.Fatalf("SetPathMapping: %v", err)
	}
	if err := c.RemovePathMappingEntry("/tmp/ws.yaml", "default", "DB_URL"); err != nil {
		t.Fatalf("RemovePathMappingEntry: %v", err)
	}
	_, found, err := c.GetPathMapping("/tmp/ws.yaml", "default", "map1")
	if err != nil {
		t.Fatalf("GetPathMapping: %v", err)
	}
	if found {
		t.Fatal("expected empty path mapping blob to be deleted")
	}
}

func TestManagerDeleteAndPathMappingHelpers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	mgr, err := NewManager(&GlobalConfig{Cache: CacheConfig{Enabled: true, TTL: time.Hour}})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })

	configPath := filepath.Join(t.TempDir(), "ws.yaml")
	if err := os.WriteFile(configPath, []byte("default:\n  provider: local\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := mgr.Set(configPath, "default", "FOO", "old", time.Hour); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := mgr.Delete(configPath, "default", "FOO"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, found, err := mgr.Get(configPath, "default", "FOO")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found {
		t.Fatal("expected FOO to be deleted via Manager")
	}

	mapping := CachedPathMapping{
		Entries: []CachedPathEntry{
			{EnvironmentVariable: "BAR", SourceRef: "bar", Value: "one"},
			{EnvironmentVariable: "BAZ", SourceRef: "baz", Value: "two"},
		},
	}
	if err := mgr.SetPathMapping(configPath, "default", "map1", mapping, time.Hour); err != nil {
		t.Fatalf("SetPathMapping: %v", err)
	}
	if err := mgr.PatchPathMappingEntry(configPath, "default", "BAR", "patched"); err != nil {
		t.Fatalf("PatchPathMappingEntry: %v", err)
	}
	got, found, err := mgr.GetPathMapping(configPath, "default", "map1")
	if err != nil {
		t.Fatalf("GetPathMapping: %v", err)
	}
	if !found {
		t.Fatal("expected path mapping")
	}
	values := map[string]string{}
	for _, e := range got.Entries {
		values[e.EnvironmentVariable] = e.Value
	}
	if values["BAR"] != "patched" {
		t.Fatalf("BAR = %q, want patched", values["BAR"])
	}

	if err := mgr.RemovePathMappingEntry(configPath, "default", "BAZ"); err != nil {
		t.Fatalf("RemovePathMappingEntry: %v", err)
	}
	got, found, err = mgr.GetPathMapping(configPath, "default", "map1")
	if err != nil {
		t.Fatalf("GetPathMapping: %v", err)
	}
	if !found {
		t.Fatal("expected path mapping to remain after removing one entry")
	}
	if len(got.Entries) != 1 || got.Entries[0].EnvironmentVariable != "BAR" {
		t.Fatalf("unexpected entries after remove: %+v", got.Entries)
	}
}

func TestManagerDisabledIsNoop(t *testing.T) {
	mgr, err := NewManager(&GlobalConfig{Cache: CacheConfig{Enabled: false, TTL: time.Hour}})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })

	if err := mgr.Delete("/tmp/ws.yaml", "default", "FOO"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := mgr.PatchPathMappingEntry("/tmp/ws.yaml", "default", "FOO", "x"); err != nil {
		t.Fatalf("PatchPathMappingEntry: %v", err)
	}
	if err := mgr.RemovePathMappingEntry("/tmp/ws.yaml", "default", "FOO"); err != nil {
		t.Fatalf("RemovePathMappingEntry: %v", err)
	}
}

func TestGetCacheDirPrefersWithsecretsPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	got, err := getCacheDir()
	if err != nil {
		t.Fatalf("getCacheDir: %v", err)
	}
	want := filepath.Join(home, ".cache", "withsecrets")
	if got != want {
		t.Fatalf("getCacheDir() = %q, want %q", got, want)
	}
	if err := os.MkdirAll(got, 0755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
}
