package ws

import (
	"testing"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
)

func TestConfigDefaultsSetAndClear(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Set defaults for gcp.
	_ = configDefaultsSetCmd.Flags().Set("provider", "gcp")
	_ = configDefaultsSetCmd.Flags().Set("regions", "us-central1,europe-west1")
	_ = configDefaultsSetCmd.Flags().Set("clear", "false")
	if err := configDefaultsSetCmd.RunE(configDefaultsSetCmd, []string{}); err != nil {
		t.Fatalf("config defaults set failed: %v", err)
	}

	gc, err := config.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("load global config failed: %v", err)
	}
	if gc.Defaults == nil || gc.Defaults.Providers == nil {
		t.Fatalf("expected defaults.providers to be set")
	}
	regions := gc.Defaults.Providers["gcp"].Regions
	if len(regions) != 2 || regions[0] != "us-central1" || regions[1] != "europe-west1" {
		t.Fatalf("unexpected regions after set: %#v", regions)
	}

	// Clear defaults for gcp.
	_ = configDefaultsSetCmd.Flags().Set("provider", "gcp")
	_ = configDefaultsSetCmd.Flags().Set("clear", "true")
	if err := configDefaultsSetCmd.RunE(configDefaultsSetCmd, []string{}); err != nil {
		t.Fatalf("config defaults clear failed: %v", err)
	}

	gc, err = config.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("load global config failed: %v", err)
	}
	if gc.Defaults != nil && gc.Defaults.Providers != nil {
		if _, ok := gc.Defaults.Providers["gcp"]; ok {
			t.Fatalf("expected gcp defaults to be cleared")
		}
	}
}

func TestConfigDefaultsSetRequiresProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_ = configDefaultsSetCmd.Flags().Set("provider", "")
	_ = configDefaultsSetCmd.Flags().Set("regions", "us-central1")
	_ = configDefaultsSetCmd.Flags().Set("clear", "false")
	if err := configDefaultsSetCmd.RunE(configDefaultsSetCmd, []string{}); err == nil {
		t.Fatalf("expected error when provider is empty")
	}
}
