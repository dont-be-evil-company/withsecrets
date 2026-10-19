package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mistweaverco/withsecrets/apps/server/storage"
)

// Config is process configuration. Secrets come from the environment, never from the binary.
type Config struct {
	PublicBaseURL   string
	DevAuth         bool
	ListenAddr      string
	DatabaseURL     string
	AttachmentDir   string
	GitHubID        string
	GitHubSecret    string
	GoogleID        string
	GoogleSecret    string
	MicrosoftID     string
	MicrosoftSecret string
	MicrosoftTenant string
	AccessTTL       time.Duration
	RefreshTTL      time.Duration
	GrantTTL        time.Duration
	AllowedHosts    []string
}

func (c Config) withDefaults() Config {
	if c.AccessTTL == 0 {
		c.AccessTTL = 15 * time.Minute
	}
	if c.RefreshTTL == 0 {
		c.RefreshTTL = 30 * 24 * time.Hour
	}
	if c.GrantTTL == 0 {
		c.GrantTTL = 60 * time.Second
	}
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	return c
}

// Validate rejects public HTTP and development auth outside loopback.
func (c Config) Validate() error {
	c = c.withDefaults()
	u, err := url.Parse(c.PublicBaseURL)
	if err != nil || u.Host == "" {
		return errors.New("PUBLIC_BASE_URL is required")
	}
	loop := isLoopback(u.Hostname())
	switch u.Scheme {
	case "https":
	case "http":
		if !loop {
			return errors.New("HTTP is only allowed for loopback development")
		}
	default:
		return errors.New("PUBLIC_BASE_URL scheme must be https")
	}
	if c.DevAuth && !loop {
		return errors.New("WITHSECRETS_DEV_AUTH requires a loopback PUBLIC_BASE_URL")
	}
	return nil
}

func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// ProviderNames returns configured identity providers. Empty means fail closed.
func (c Config) ProviderNames() []string {
	var out []string
	if c.GitHubID != "" && c.GitHubSecret != "" {
		out = append(out, "github")
	}
	if c.GoogleID != "" && c.GoogleSecret != "" {
		out = append(out, "google")
	}
	if c.MicrosoftID != "" && c.MicrosoftSecret != "" && c.MicrosoftTenant != "" && c.MicrosoftTenant != "common" {
		out = append(out, "microsoft")
	}
	if c.DevAuth {
		out = append(out, "dev")
	}
	return out
}

// Server is the vault HTTP API and the static portal host.
type Server struct {
	cfg    Config
	store  storage.Store
	mux    *http.ServeMux
	log    io.Writer
	client *http.Client
	oauth  map[string]oauthProvider
	portal http.Handler
}

// New builds the HTTP handler. portal may be nil; a built-in page is used then.
func New(cfg Config, store storage.Store, portal http.Handler) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg = cfg.withDefaults()
	s := &Server{
		cfg:    cfg,
		store:  store,
		mux:    http.NewServeMux(),
		log:    io.Discard,
		client: &http.Client{Timeout: 15 * time.Second},
		portal: portal,
	}
	s.oauth = s.providers()
	s.routes()
	return s, nil
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) SetLog(w io.Writer) { s.log = w }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("GET /readyz", s.ready)
	s.mux.HandleFunc("GET /.well-known/withsecrets", s.wellKnown)
	s.mux.HandleFunc("GET /auth/providers", s.authProviders)
	s.mux.HandleFunc("GET /auth/desktop/start", s.desktopStart)
	s.mux.HandleFunc("GET /auth/desktop/continue", s.desktopContinue)
	s.mux.HandleFunc("GET /auth/callback/{provider}", s.authCallback)
	s.mux.HandleFunc("POST /api/v1/auth/desktop/exchange", s.exchange)
	s.mux.HandleFunc("POST /api/v1/auth/refresh", s.refresh)
	s.mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	s.mux.HandleFunc("GET /api/v1/accounts/me", s.me)
	s.mux.HandleFunc("POST /api/v1/accounts/enroll", s.enroll)
	s.mux.HandleFunc("GET /api/v1/accounts/unlock-slots", s.listSlots)
	s.mux.HandleFunc("POST /api/v1/accounts/unlock-slots", s.createSlot)
	s.mux.HandleFunc("POST /api/v1/accounts/unlock-slots/{id}/verify", s.verifySlot)
	s.mux.HandleFunc("DELETE /api/v1/accounts/unlock-slots/{id}", s.deleteSlot)
	s.mux.HandleFunc("GET /api/v1/accounts/key-generations", s.listGenerations)
	s.mux.HandleFunc("POST /api/v1/accounts/key-rotations", s.rotateKeys)
	s.mux.HandleFunc("GET /api/v1/devices", s.listDevices)
	s.mux.HandleFunc("POST /api/v1/devices/enrollment-request", s.enrollDevice)
	s.mux.HandleFunc("POST /api/v1/devices/{id}/approve", s.approveDevice)
	s.mux.HandleFunc("DELETE /api/v1/devices/{id}", s.revokeDevice)
	s.mux.HandleFunc("GET /api/v1/vaults", s.listVaults)
	s.mux.HandleFunc("POST /api/v1/vaults", s.createVault)
	s.mux.HandleFunc("GET /api/v1/vaults/{id}", s.getVault)
	s.mux.HandleFunc("PUT /api/v1/vaults/{id}/account-key", s.putAccountKey)
	s.mux.HandleFunc("DELETE /api/v1/vaults/{id}", s.deleteVault)
	s.mux.HandleFunc("GET /api/v1/vaults/{id}/items", s.listItems)
	s.mux.HandleFunc("POST /api/v1/vaults/{id}/items", s.createItem)
	s.mux.HandleFunc("GET /api/v1/items/{id}", s.getItem)
	s.mux.HandleFunc("PUT /api/v1/items/{id}", s.putItem)
	s.mux.HandleFunc("DELETE /api/v1/items/{id}", s.deleteItem)
	s.mux.HandleFunc("GET /api/v1/items/{id}/history", s.itemHistory)
	s.mux.HandleFunc("GET /api/v1/organizations", s.listOrgs)
	s.mux.HandleFunc("POST /api/v1/organizations", s.createOrg)
	s.mux.HandleFunc("POST /api/v1/organizations/{id}/invitations", s.invite)
	s.mux.HandleFunc("GET /api/v1/organizations/{id}/members", s.listOrgMembers)
	s.mux.HandleFunc("POST /api/v1/invitations/{token}/accept", s.acceptInvite)
	s.mux.HandleFunc("POST /api/v1/vaults/{id}/members", s.addMember)
	s.mux.HandleFunc("DELETE /api/v1/vaults/{id}/members/{principalId}", s.removeMember)
	s.mux.HandleFunc("GET /api/v1/vaults/{id}/key-envelopes", s.listEnvelopes)
	s.mux.HandleFunc("POST /api/v1/vaults/{id}/key-envelopes", s.putEnvelope)
	s.mux.HandleFunc("POST /api/v1/vaults/{id}/rotations", s.rotateVault)
	s.mux.HandleFunc("POST /api/v1/attachments/uploads", s.startUpload)
	s.mux.HandleFunc("PUT /api/v1/attachments/uploads/{id}/chunks/{n}", s.putChunk)
	s.mux.HandleFunc("POST /api/v1/attachments/uploads/{id}/complete", s.completeUpload)
	s.mux.HandleFunc("GET /api/v1/attachments/{id}/chunks/{n}", s.getChunk)
	s.mux.HandleFunc("GET /api/v1/sync/changes", s.changes)
	s.mux.HandleFunc("GET /api/v1/sync/events", s.events)
	s.mux.HandleFunc("GET /api/v1/audit/events", s.audit)
	s.mux.HandleFunc("/", s.portalPage)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ready(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not-ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) wellKnown(w http.ResponseWriter, r *http.Request) {
	id, err := s.store.InstanceID(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "instance")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"product":       "withsecrets",
		"apiVersion":    "v1",
		"instanceId":    id,
		"publicBaseUrl": s.cfg.PublicBaseURL,
		"authProviders": s.cfg.ProviderNames(),
		"cryptoVersion": 1,
		"features":      []string{"personal-vault", "hardware-slots", "team-vaults", "attachments"},
	})
}

func (s *Server) authProviders(w http.ResponseWriter, _ *http.Request) {
	names := s.cfg.ProviderNames()
	if len(names) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "setup_required", "providers": []string{},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": names})
}

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	token := bearerOrCookie(r)
	if token == "" {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	sess, err := s.store.SessionByAccess(r.Context(), hashToken(token))
	if err != nil || time.Now().After(sess.AccessExp) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	return sess.UserID, true
}

func bearerOrCookie(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		token := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		if token != "" {
			return token
		}
	}
	c, err := r.Cookie("ws_access")
	if err != nil {
		return ""
	}
	return c.Value
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, kind string) {
	writeJSON(w, code, map[string]string{"error": kind})
}

func randomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func s256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return errors.New("empty")
	}
	return json.Unmarshal(body, dst)
}

var bannedFields = map[string]struct{}{
	"password": {}, "passphrase": {}, "totp": {}, "seed": {}, "privatekey": {},
	"masterkey": {}, "title": {}, "username": {}, "notes": {}, "secret": {}, "pin": {},
}

func rejectPlaintext(body []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	for k := range raw {
		if _, ok := bannedFields[strings.ToLower(k)]; ok {
			return errors.New("plaintext field")
		}
	}
	return nil
}
