package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dont-be-evil-company/withsecrets/internal/guiapi"
)

const (
	defaultPort    = 11911
	maxPortRetries = 10
)

type Options struct {
	ConfigPath string
	NoBrowser  bool
}

type errorResponse struct {
	Error string `json:"error"`
}

func Run(ctx context.Context, opts Options) error {
	webDir, err := ensureAssetsExtracted()
	if err != nil {
		return err
	}

	port, err := findAvailablePort(defaultPort, maxPortRetries)
	if err != nil {
		return err
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	url := "http://" + addr

	mux := http.NewServeMux()
	auth, err := newGUIAuth(port)
	if err != nil {
		return fmt.Errorf("failed to initialize gui auth: %w", err)
	}
	registerAPIRoutes(mux, opts.ConfigPath, auth)
	mux.Handle("/", localhostStaticHandler(webDir))

	server := &http.Server{
		Addr:    addr,
		Handler: auth.middleware(mux),
	}

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	fmt.Fprintf(os.Stdout, "GUI available at %s\n", url)
	if !opts.NoBrowser {
		if err := openBrowser(url); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not open browser: %v\n", err)
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case sig := <-sigCh:
		fmt.Fprintf(os.Stdout, "\nReceived %s, shutting down GUI server...\n", sig)
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

func findAvailablePort(start, count int) (int, error) {
	for i := 0; i < count; i++ {
		port := start + i
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port in range %d-%d", start, start+count-1)
}

func localhostStaticHandler(webDir string) http.Handler {
	fileServer := http.FileServer(http.Dir(webDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			http.ServeFile(w, r, webDir+"/index.html")
			return
		}
		full := strings.Join([]string{webDir, path}, string(os.PathSeparator))
		if _, err := os.Stat(full); os.IsNotExist(err) {
			http.ServeFile(w, r, webDir+"/index.html")
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func registerAPIRoutes(mux *http.ServeMux, configPath string, auth *guiAuth) {
	mux.HandleFunc("GET /api/auth/token", auth.handleToken)

	guard := auth.apiGuard
	mux.Handle("GET /api/environments", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envs, err := guiapi.ListEnvironments(configPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"environments": envs})
	})))

	mux.Handle("GET /api/environments/{env}/secrets", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env := r.PathValue("env")
		rows, err := guiapi.ListSecrets(r.Context(), configPath, env)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"secrets": rows})
	})))

	mux.Handle("GET /api/environments/{env}/secrets/{var}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env := r.PathValue("env")
		envVar := r.PathValue("var")
		val, err := guiapi.GetSecret(r.Context(), configPath, env, envVar)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"envVar": envVar, "value": val})
	})))

	mux.Handle("POST /api/environments/{env}/secrets", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env := r.PathValue("env")
		var body struct {
			EnvVar      string   `json:"envVar"`
			Kind        string   `json:"kind"`
			SecretKey   string   `json:"secretKey"`
			Paths       []string `json:"paths"`
			Value       string   `json:"value"`
			Description string   `json:"description"`
			Replication string   `json:"replication"`
			Locations   []string `json:"locations"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		err := guiapi.CreateSecret(r.Context(), guiapi.CreateInput{
			ConfigPath:  configPath,
			EnvName:     env,
			EnvVar:      body.EnvVar,
			Kind:        body.Kind,
			SecretKey:   body.SecretKey,
			Paths:       body.Paths,
			Value:       body.Value,
			Description: body.Description,
			Replication: body.Replication,
			Locations:   body.Locations,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})))

	mux.Handle("PUT /api/environments/{env}/secrets/{var}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env := r.PathValue("env")
		envVar := r.PathValue("var")
		var body struct {
			Value string   `json:"value"`
			Paths []string `json:"paths"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		var err error
		if body.Paths != nil {
			err = guiapi.UpdatePathMapping(r.Context(), configPath, env, envVar, body.Paths)
		} else {
			err = guiapi.UpdateSecret(r.Context(), configPath, env, envVar, body.Value)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))

	mux.Handle("DELETE /api/environments/{env}/secrets/{var}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env := r.PathValue("env")
		envVar := r.PathValue("var")
		if err := guiapi.DeleteSecret(r.Context(), configPath, env, envVar); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))

	mux.Handle("GET /api/environments/{env}/create-options", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env := r.PathValue("env")
		opts, err := guiapi.GetCreateOptions(r.Context(), configPath, env)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, opts)
	})))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errorResponse{Error: err.Error()})
}

// DrainBody closes request bodies to avoid leaks in tests.
func DrainBody(r *http.Request) {
	if r.Body != nil {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	}
}
