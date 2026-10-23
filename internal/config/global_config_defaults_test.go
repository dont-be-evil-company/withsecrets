package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGlobalConfigParsesDefaultsProvidersRegions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgDir := filepath.Join(home, ".config", "withsecrets")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	content := []byte(`
cache:
  enabled: true
  ttl: 1d
defaults:
  providers:
    gcp:
      regions:
        - us-central1
        - europe-west1
    aws:
      regions:
        - eu-central-1
`)
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), content, 0644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	gc, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig() error: %v", err)
	}
	if gc.Defaults == nil || gc.Defaults.Providers == nil {
		t.Fatalf("expected defaults.providers to be parsed")
	}
	gcp := gc.Defaults.Providers["gcp"]
	if len(gcp.Regions) != 2 || gcp.Regions[0] != "us-central1" || gcp.Regions[1] != "europe-west1" {
		t.Fatalf("unexpected gcp regions: %#v", gcp.Regions)
	}
	aws := gc.Defaults.Providers["aws"]
	if len(aws.Regions) != 1 || aws.Regions[0] != "eu-central-1" {
		t.Fatalf("unexpected aws regions: %#v", aws.Regions)
	}
}

func TestSaveGlobalConfigPersistsDefaultsProvidersRegions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	gc := &GlobalConfig{
		Defaults: &DefaultsConfig{
			Providers: map[string]ProviderDefaults{
				"gcp": {Regions: []string{"us-central1", "europe-west1"}},
			},
		},
	}
	if err := SaveGlobalConfig(gc); err != nil {
		t.Fatalf("SaveGlobalConfig() error: %v", err)
	}

	loaded, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig() error: %v", err)
	}
	if loaded.Defaults == nil || loaded.Defaults.Providers == nil {
		t.Fatalf("expected defaults.providers after save+load")
	}
	regions := loaded.Defaults.Providers["gcp"].Regions
	if len(regions) != 2 || regions[0] != "us-central1" || regions[1] != "europe-west1" {
		t.Fatalf("unexpected loaded regions: %#v", regions)
	}
}
