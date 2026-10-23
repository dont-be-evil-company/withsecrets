package ws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
)

func TestParseKsvcFile_ParsesEnvAndSecrets(t *testing.T) {
	tmpDir := t.TempDir()
	ksvcPath := filepath.Join(tmpDir, "ksvc.yaml")

	ksvcContent := `
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: api-iam-prod
  namespace: "4467360136"
spec:
  template:
    spec:
      containers:
      - image: example
        env:
        - name: GCP_REGION
          value: europe-west3
        - name: GCP_PROJECT
          value: api-infra
        - name: DB_PORT
          value: "5432"
        - name: SALT
          valueFrom:
            secretKeyRef:
              key: latest
              name: api-iam-salt
        - name: JWT_SECRET
          valueFrom:
            secretKeyRef:
              key: latest
              name: api-jwt-secret
`

	if err := os.WriteFile(ksvcPath, []byte(ksvcContent), 0o644); err != nil {
		t.Fatalf("failed to write temp ksvc file: %v", err)
	}

	items, provider, project, err := parseKsvcFile(ksvcPath)
	if err != nil {
		t.Fatalf("parseKsvcFile returned error: %v", err)
	}

	if provider != "gcp" {
		t.Fatalf("expected provider 'gcp', got %q", provider)
	}

	if project != "4467360136" {
		t.Fatalf("expected project '4467360136', got %q", project)
	}

	// Hard-coded values
	expectValue := map[string]string{
		"GCP_REGION":  "europe-west3",
		"GCP_PROJECT": "api-infra",
		"DB_PORT":     "5432",
	}

	for key, expected := range expectValue {
		item, ok := items[key]
		if !ok {
			t.Fatalf("expected env item %q to be present", key)
		}
		if item.Value == nil {
			t.Fatalf("expected env item %q to have value, got nil", key)
		}
		if got := item.Value.(string); got != expected {
			t.Fatalf("env item %q: expected value %q, got %q", key, expected, got)
		}
		if item.SecretKey != "" || len(item.SecretPath) > 0 {
			t.Fatalf("env item %q: expected no secret fields, got secret-key=%q secret-path=%q", key, item.SecretKey, item.SecretPath)
		}
	}

	// Secret-backed env vars
	secretExpectations := map[string]string{
		"SALT":       "api-iam-salt",
		"JWT_SECRET": "api-jwt-secret",
	}

	for key, expectedSecret := range secretExpectations {
		item, ok := items[key]
		if !ok {
			t.Fatalf("expected env item %q to be present", key)
		}
		if item.SecretKey != expectedSecret {
			t.Fatalf("env item %q: expected secret-key %q, got %q", key, expectedSecret, item.SecretKey)
		}
		if item.Value != nil {
			t.Fatalf("env item %q: expected nil value for secret-backed var, got %#v", key, item.Value)
		}
	}
}

// Basic sanity check to ensure we can merge ksvc-derived env items into a config.Environment.
func TestKsvcItemsMergeIntoEnvironment(t *testing.T) {
	items := map[string]config.EnvItem{
		"FOO": {Value: "bar"},
		"BAZ": {SecretKey: "secret-id"},
	}

	env := config.Environment{
		Provider: "gcp",
		Project:  "1234",
		Env:      map[string]config.EnvItem{},
	}

	for k, v := range items {
		env.Env[k] = v
	}

	if len(env.Env) != 2 {
		t.Fatalf("expected 2 env items, got %d", len(env.Env))
	}

	if env.Env["FOO"].Value != "bar" {
		t.Fatalf("expected FOO value 'bar', got %#v", env.Env["FOO"].Value)
	}

	if env.Env["BAZ"].SecretKey != "secret-id" {
		t.Fatalf("expected BAZ secret-key 'secret-id', got %q", env.Env["BAZ"].SecretKey)
	}
}

func TestParseAWSServiceAndRegion(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantSvc    string
		wantRegion string
		wantErr    bool
	}{
		{
			name:       "valid service and region",
			input:      "my-service.us-east-1",
			wantSvc:    "my-service",
			wantRegion: "us-east-1",
			wantErr:    false,
		},
		{
			name:       "service name with dots",
			input:      "api.my-service.eu-west-1",
			wantSvc:    "api.my-service",
			wantRegion: "eu-west-1",
			wantErr:    false,
		},
		{
			name:    "missing region",
			input:   "my-service",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSvc, gotRegion, err := parseAWSServiceAndRegion(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotSvc != tt.wantSvc {
				t.Fatalf("service: expected %q, got %q", tt.wantSvc, gotSvc)
			}
			if gotRegion != tt.wantRegion {
				t.Fatalf("region: expected %q, got %q", tt.wantRegion, gotRegion)
			}
		})
	}
}

func TestParseAzureAppAndResourceGroup(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantApp string
		wantRG  string
		wantErr bool
	}{
		{
			name:    "valid app and resource group",
			input:   "my-app.my-resource-group",
			wantApp: "my-app",
			wantRG:  "my-resource-group",
			wantErr: false,
		},
		{
			name:    "missing resource group",
			input:   "my-app",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotApp, gotRG, err := parseAzureAppAndResourceGroup(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotApp != tt.wantApp {
				t.Fatalf("app: expected %q, got %q", tt.wantApp, gotApp)
			}
			if gotRG != tt.wantRG {
				t.Fatalf("resource group: expected %q, got %q", tt.wantRG, gotRG)
			}
		})
	}
}
