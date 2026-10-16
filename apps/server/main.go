package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mistweaverco/withsecrets/apps/server/api"
	"github.com/mistweaverco/withsecrets/apps/server/storage"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get("http://127.0.0.1:8080/healthz")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	cfg := api.Config{
		PublicBaseURL:   os.Getenv("PUBLIC_BASE_URL"),
		DevAuth:         os.Getenv("WITHSECRETS_DEV_AUTH") == "1",
		ListenAddr:      envOr("LISTEN_ADDR", ":8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		AttachmentDir:   envOr("ATTACHMENT_DIR", "/var/lib/withsecrets/attachments"),
		GitHubID:        os.Getenv("OAUTH_GITHUB_CLIENT_ID"),
		GitHubSecret:    os.Getenv("OAUTH_GITHUB_CLIENT_SECRET"),
		GoogleID:        os.Getenv("OAUTH_GOOGLE_CLIENT_ID"),
		GoogleSecret:    os.Getenv("OAUTH_GOOGLE_CLIENT_SECRET"),
		MicrosoftID:     os.Getenv("OAUTH_MICROSOFT_CLIENT_ID"),
		MicrosoftSecret: os.Getenv("OAUTH_MICROSOFT_CLIENT_SECRET"),
		MicrosoftTenant: os.Getenv("OAUTH_MICROSOFT_TENANT"),
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("configuration: %v", err)
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	ctx := context.Background()
	store, err := storage.OpenPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer store.Close()
	srv, err := api.New(cfg, store, portalStatic())
	if err != nil {
		log.Fatalf("server: %v", err)
	}
	log.Printf("withsecrets server listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
