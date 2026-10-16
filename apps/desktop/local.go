package main

import (
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/mistweaverco/withsecrets/apps/desktop/app"
	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
)

type host struct {
	svc              *app.Service
	slot             vaultcrypto.UnlockSlot
	recoverySlot     vaultcrypto.UnlockSlot
	fido             []fidoRecord
	have             bool
	haveRecovery     bool
	policyChoice     string
	loginMu          sync.Mutex
	pending          pendingLogin
	session          accountSession
	loginSrv         *http.Server
	syncMu           sync.Mutex
	slotsPulled      bool
	passUploaded     bool
	recoveryUploaded bool
}

type fidoRecord struct {
	path    string
	serial  string
	cred    []byte
	slot    vaultcrypto.UnlockSlot
	removed bool
	proof   vaultcrypto.TokenProof
}

func newHost() *host {
	svc := app.New()
	svc.SetHardwareTags(builtFIDO2, builtPIV)
	if raw, err := loadSavedServer(); err == nil && raw != "" {
		_ = svc.SetDefaultServer(raw)
	}
	if saved, err := loadSavedSettings(); err == nil {
		_ = svc.ApplySettings(saved)
	}
	h := &host{svc: svc}
	h.loadVault()
	h.loadSession()
	return h
}

func (h *host) watchIdle() {
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		n := 0
		for now := range ticker.C {
			if h.svc.IdleExpired(now) {
				h.svc.Lock()
			}
			n++
			if n%2 == 0 {
				h.syncNow()
			}
		}
	}()
}

func (h *host) routes() http.Handler {
	return localhost(h.api())
}

func (h *host) api() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/local/status", h.status)
	mux.HandleFunc("POST /api/local/server", h.setServer)
	mux.HandleFunc("POST /api/local/bootstrap", h.bootstrap)
	mux.HandleFunc("POST /api/local/setup", h.setup)
	mux.HandleFunc("POST /api/local/unlock", h.unlock)
	mux.HandleFunc("POST /api/local/lock", h.lock)
	mux.HandleFunc("GET /api/local/items", h.items)
	mux.HandleFunc("POST /api/local/items", h.create)
	mux.HandleFunc("GET /api/local/items/{id}", h.item)
	mux.HandleFunc("PUT /api/local/items/{id}", h.update)
	mux.HandleFunc("DELETE /api/local/items/{id}", h.remove)
	mux.HandleFunc("POST /api/local/totp", h.totp)
	mux.HandleFunc("GET /api/local/settings", h.getSettings)
	mux.HandleFunc("POST /api/local/settings", h.setSettings)
	mux.HandleFunc("POST /api/local/activity", h.activity)
	mux.HandleFunc("POST /api/local/sign-in", h.signIn)
	mux.HandleFunc("POST /api/local/sign-out", h.signOut)
	mux.HandleFunc("POST /api/local/vaults/active", h.selectVault)
	mux.HandleFunc("POST /api/local/teams", h.createTeam)
	mux.HandleFunc("POST /api/local/teams/invite", h.inviteTeam)
	mux.HandleFunc("POST /api/local/teams/accept", h.acceptTeam)
	mux.HandleFunc("GET /api/local/teams/{orgId}/members", h.teamMembers)
	mux.HandleFunc("POST /api/local/teams/grant", h.grantTeam)
	mux.HandleFunc("POST /api/local/teams/remove", h.removeTeamMember)
	mux.HandleFunc("POST /api/local/reveal", h.reveal)
	mux.HandleFunc("POST /api/local/generate", h.generate)
	mux.HandleFunc("GET /api/local/doctor", h.doctor)
	mux.HandleFunc("GET /api/local/fido2/devices", h.fidoDevices)
	mux.HandleFunc("POST /api/local/fido2/enroll", h.fidoEnroll)
	mux.HandleFunc("POST /api/local/fido2/unlock", h.fidoUnlock)
	mux.Handle("GET /", desktopStatic())
	return mux
}

// localAPI serves the desktop API from inside the Wails asset host.
// The asset host is already process-local, so this path skips the loopback check.
func localAPI(h *host) func(http.Handler) http.Handler {
	api := h.api()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/local/") {
				api.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func localhost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || (host != "127.0.0.1" && host != "::1") {
			http.Error(w, "loopback only", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; frame-ancestors 'none'")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (h *host) setServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := read(r, &body); err != nil {
		http.Error(w, "Enter an https server URL. http is only allowed for localhost.", http.StatusBadRequest)
		return
	}
	normalized, err := app.NormalizeServerURL(body.URL)
	if err != nil {
		http.Error(w, "Enter an https server URL. http is only allowed for localhost.", http.StatusBadRequest)
		return
	}
	if err := h.svc.SetDefaultServer(normalized); err != nil {
		http.Error(w, "Enter an https server URL. http is only allowed for localhost.", http.StatusBadRequest)
		return
	}
	if err := saveServer(normalized); err != nil {
		http.Error(w, "The server was updated for this session, but it could not be saved.", http.StatusInternalServerError)
		return
	}
	write(w, map[string]any{"url": normalized, "unlocked": h.svc.Unlocked(), "profiles": h.svc.GetServerProfiles()})
}

func (h *host) status(w http.ResponseWriter, _ *http.Request) {
	signedIn, account := h.accountView()
	if signedIn {
		h.pullSlots()
	}
	syncStatus := h.svc.SyncStatus()
	if !signedIn {
		syncStatus = "off"
	}
	write(w, map[string]any{
		"unlocked": h.svc.Unlocked(), "bootstrapped": h.have || h.haveRecovery || len(h.fido) > 0,
		"passphrase": h.have, "recovery": h.haveRecovery, "securityKeys": len(h.fido),
		"profiles": h.svc.GetServerProfiles(),
		"settings": h.svc.Settings(), "signedIn": signedIn, "account": account,
		"sync": syncStatus, "vaults": h.svc.VaultChoices(), "activeVault": h.svc.ActiveVault(),
	})
}

func (h *host) getSettings(w http.ResponseWriter, _ *http.Request) {
	write(w, h.svc.Settings())
}

func (h *host) setSettings(w http.ResponseWriter, r *http.Request) {
	var body app.DesktopSettings
	if err := read(r, &body); err != nil {
		http.Error(w, "Choose a dark or light theme. Idle lock is 0 to 240 minutes.", http.StatusBadRequest)
		return
	}
	if err := h.svc.ApplySettings(body); err != nil {
		http.Error(w, "Choose a dark or light theme. Idle lock is 0 to 240 minutes.", http.StatusBadRequest)
		return
	}
	saved := h.svc.Settings()
	if err := saveSettings(saved); err != nil {
		http.Error(w, "Settings were updated for this session, but they could not be saved.", http.StatusInternalServerError)
		return
	}
	write(w, saved)
}

func (h *host) activity(w http.ResponseWriter, _ *http.Request) {
	h.svc.Touch()
	write(w, map[string]bool{"ok": true})
}

func (h *host) item(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.GetItem(r.PathValue("id"))
	if err != nil {
		http.Error(w, "locked", http.StatusUnauthorized)
		return
	}
	write(w, item)
}

func (h *host) update(w http.ResponseWriter, r *http.Request) {
	var item app.ItemBody
	if err := read(r, &item); err != nil {
		http.Error(w, "item", http.StatusBadRequest)
		return
	}
	item.ID = r.PathValue("id")
	if _, err := h.svc.PutDecryptedItem(item); err != nil {
		http.Error(w, "locked", http.StatusConflict)
		return
	}
	if !h.saved(w) {
		return
	}
	h.syncNow()
	write(w, map[string]any{"items": h.svc.Search("")})
}

func (h *host) remove(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteItem(r.PathValue("id")); err != nil {
		http.Error(w, "locked", http.StatusConflict)
		return
	}
	if !h.saved(w) {
		return
	}
	h.syncNow()
	write(w, map[string]any{"items": h.svc.Search("")})
}

func (h *host) totp(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if err := read(r, &body); err != nil {
		http.Error(w, "totp", http.StatusBadRequest)
		return
	}
	code, remain, err := h.svc.CurrentTOTP(body.ID, time.Now())
	if err != nil {
		http.Error(w, "totp", http.StatusBadRequest)
		return
	}
	write(w, map[string]any{"code": code, "seconds": remain})
}

func (h *host) bootstrap(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Passphrase string `json:"passphrase"`
	}
	if err := read(r, &body); err != nil || len(body.Passphrase) < 8 {
		http.Error(w, "passphrase", http.StatusBadRequest)
		return
	}
	slot, err := h.svc.BootstrapPassphrase(body.Passphrase)
	if err != nil {
		http.Error(w, "bootstrap", http.StatusBadRequest)
		return
	}
	h.slot = slot
	h.have = true
	if !h.saved(w) {
		return
	}
	h.afterUnlock(body.Passphrase, "")
	write(w, map[string]any{"unlocked": true})
}

func (h *host) setup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Policy     string `json:"policy"`
		Passphrase string `json:"passphrase"`
	}
	if err := read(r, &body); err != nil {
		http.Error(w, "Setup could not finish. If a recovery key already exists, unlock with that key.", http.StatusBadRequest)
		return
	}
	if body.Policy == "passphrase" && len(strings.TrimSpace(body.Passphrase)) < 8 {
		http.Error(w, "Enter a passphrase of at least 8 characters, then choose Passphrase and recovery key.", http.StatusBadRequest)
		return
	}
	res, err := h.svc.BeginSetup(body.Policy, body.Passphrase)
	if err != nil {
		http.Error(w, "Setup could not finish. If a recovery key already exists, unlock with that key.", http.StatusBadRequest)
		return
	}
	if res.HasPass {
		h.slot = res.PassSlot
		h.have = true
	}
	h.recoverySlot = res.RecSlot
	h.haveRecovery = true
	if !h.saved(w) {
		return
	}
	h.afterUnlock(body.Passphrase, res.RecoveryKey)
	write(w, map[string]any{
		"unlocked": true, "policy": res.Policy, "recoveryKey": res.RecoveryKey,
		"hardware": h.svc.Doctor(),
	})
}

func (h *host) unlock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Passphrase  string `json:"passphrase"`
		RecoveryKey string `json:"recoveryKey"`
	}
	if err := read(r, &body); err != nil {
		http.Error(w, "unlock", http.StatusBadRequest)
		return
	}
	if body.RecoveryKey != "" {
		if !h.haveRecovery {
			http.Error(w, "unlock", http.StatusBadRequest)
			return
		}
		if err := h.svc.UnlockWithRecovery(body.RecoveryKey, h.recoverySlot); err != nil {
			http.Error(w, "unlock failed", http.StatusUnauthorized)
			return
		}
		h.afterUnlock("", body.RecoveryKey)
		write(w, map[string]any{"unlocked": true})
		return
	}
	if !h.have {
		http.Error(w, "unlock", http.StatusBadRequest)
		return
	}
	if err := h.svc.UnlockWithPassphrase(body.Passphrase, h.slot); err != nil {
		http.Error(w, "unlock failed", http.StatusUnauthorized)
		return
	}
	h.afterUnlock(body.Passphrase, "")
	write(w, map[string]any{"unlocked": true})
}

func (h *host) lock(w http.ResponseWriter, _ *http.Request) {
	h.svc.Lock()
	write(w, map[string]any{"unlocked": false})
}

func (h *host) items(w http.ResponseWriter, r *http.Request) {
	write(w, map[string]any{"items": h.svc.Search(r.URL.Query().Get("q"))})
}

func (h *host) create(w http.ResponseWriter, r *http.Request) {
	var item app.ItemBody
	if err := read(r, &item); err != nil {
		http.Error(w, "item", http.StatusBadRequest)
		return
	}
	if _, err := h.svc.PutDecryptedItem(item); err != nil {
		http.Error(w, "locked", http.StatusConflict)
		return
	}
	if !h.saved(w) {
		return
	}
	h.syncNow()
	write(w, map[string]any{"items": h.svc.Search("")})
}

func (h *host) reveal(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID    string `json:"id"`
		Field string `json:"field"`
	}
	if err := read(r, &body); err != nil {
		http.Error(w, "reveal", http.StatusBadRequest)
		return
	}
	value, err := h.svc.RevealField(body.ID, body.Field)
	if err != nil {
		http.Error(w, "locked", http.StatusUnauthorized)
		return
	}
	write(w, map[string]string{"value": value})
}

func (h *host) generate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Length int `json:"length"`
	}
	_ = read(r, &body)
	if body.Length == 0 {
		body.Length = 20
	}
	pw, err := h.svc.GeneratePassword(body.Length)
	if err != nil {
		http.Error(w, "generate", http.StatusBadRequest)
		return
	}
	write(w, map[string]any{"password": pw, "length": len(pw)})
}

func (h *host) doctor(w http.ResponseWriter, _ *http.Request) {
	write(w, h.svc.Doctor())
}

func desktopStatic() http.Handler {
	root, err := fs.Sub(desktopFiles, "ui")
	if err != nil {
		root = desktopFiles
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		data, err := fs.ReadFile(root, name)
		if err != nil {
			data, err = fs.ReadFile(root, "index.html")
			name = "index.html"
		}
		if err != nil {
			http.NotFound(w, r)
			return
		}
		switch {
		case strings.HasSuffix(name, ".js"):
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case strings.HasSuffix(name, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case strings.HasSuffix(name, ".html"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		case strings.HasSuffix(name, ".svg"):
			w.Header().Set("Content-Type", "image/svg+xml")
		case strings.HasSuffix(name, ".png"):
			w.Header().Set("Content-Type", "image/png")
		case strings.HasSuffix(name, ".woff2"):
			w.Header().Set("Content-Type", "font/woff2")
		}
		_, _ = w.Write(data)
	})
}

func read(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(dst)
}

func write(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
