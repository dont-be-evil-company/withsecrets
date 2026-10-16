package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type pendingLogin struct {
	State    string
	Verifier string
	Server   string
}

type accountSession struct {
	Server  string    `json:"server"`
	Access  string    `json:"access"`
	Refresh string    `json:"refresh"`
	Account string    `json:"account"`
	Expires time.Time `json:"expires"`
}

func (h *host) signIn(w http.ResponseWriter, r *http.Request) {
	profiles := h.svc.GetServerProfiles()
	if len(profiles) == 0 || profiles[0].URL == "" {
		http.Error(w, "Set a server URL in Settings first.", http.StatusBadRequest)
		return
	}
	server := strings.TrimRight(profiles[0].URL, "/")
	providers, err := providerNames(server)
	if err != nil {
		http.Error(w, "Could not reach the server. Check the server URL in Settings.", http.StatusBadGateway)
		return
	}
	if len(providers) == 0 {
		http.Error(w, "This server has no identity provider configured, so sign-in is closed.", http.StatusConflict)
		return
	}
	state, err := randomToken()
	if err != nil {
		http.Error(w, "Could not start sign-in.", http.StatusInternalServerError)
		return
	}
	verifier, err := randomToken()
	if err != nil {
		http.Error(w, "Could not start sign-in.", http.StatusInternalServerError)
		return
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		http.Error(w, "Could not listen for the browser to return.", http.StatusInternalServerError)
		return
	}
	callback := fmt.Sprintf("http://127.0.0.1:%d/auth/callback", ln.Addr().(*net.TCPAddr).Port)
	h.loginMu.Lock()
	if h.loginSrv != nil {
		_ = h.loginSrv.Close()
	}
	h.pending = pendingLogin{State: state, Verifier: verifier, Server: server}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/callback", h.finishSignIn)
	h.loginSrv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	srv := h.loginSrv
	h.loginMu.Unlock()
	go func() { _ = srv.Serve(ln) }()
	time.AfterFunc(5*time.Minute, func() { _ = srv.Close() })

	q := url.Values{}
	q.Set("state", state)
	q.Set("code_challenge", s256(verifier))
	q.Set("code_challenge_method", "S256")
	q.Set("callback", callback)
	if len(providers) == 1 {
		q.Set("provider", providers[0])
	}
	start := server + "/auth/desktop/start?" + q.Encode()
	if err := openBrowser(start); err != nil {
		write(w, map[string]any{"started": true, "url": start, "browser": false})
		return
	}
	write(w, map[string]any{"started": true, "url": start, "browser": true})
}

func (h *host) finishSignIn(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || (host != "127.0.0.1" && host != "::1") {
		http.Error(w, "loopback only", http.StatusForbidden)
		return
	}
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	h.loginMu.Lock()
	pending := h.pending
	if code == "" || state == "" || state != pending.State || pending.Verifier == "" {
		h.loginMu.Unlock()
		http.Error(w, "This sign-in does not match the desktop window. Start again from withsecrets.", http.StatusBadRequest)
		return
	}
	h.pending = pendingLogin{}
	h.loginMu.Unlock()

	access, refresh, err := exchangeCode(pending.Server, code, pending.Verifier, state)
	if err != nil {
		http.Error(w, "The server rejected this sign-in. Start again from the desktop.", http.StatusBadRequest)
		return
	}
	account, err := fetchAccount(pending.Server, access)
	if err != nil {
		account = pending.Server
	}
	sess := accountSession{
		Server: pending.Server, Access: access, Refresh: refresh,
		Account: account, Expires: time.Now().Add(14 * time.Minute),
	}
	h.loginMu.Lock()
	h.session = sess
	h.loginMu.Unlock()
	_ = saveSession(sess)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>withsecrets</title></head><body><p>Signed in. Return to the withsecrets window. This sign-in does not unlock the vault.</p></body></html>`))
	h.loginMu.Lock()
	srv := h.loginSrv
	h.loginMu.Unlock()
	if srv != nil {
		go func() { _ = srv.Close() }()
	}
}

func (h *host) signOut(w http.ResponseWriter, _ *http.Request) {
	h.loginMu.Lock()
	sess := h.session
	h.session = accountSession{}
	h.pending = pendingLogin{}
	h.loginMu.Unlock()
	h.slotsPulled = false
	h.passUploaded = false
	h.recoveryUploaded = false
	if sess.Access != "" {
		req, err := http.NewRequest(http.MethodPost, sess.Server+"/api/v1/auth/logout", nil)
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+sess.Access)
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err == nil {
				_ = resp.Body.Close()
			}
		}
	}
	_ = saveSession(accountSession{})
	write(w, map[string]bool{"signedIn": false})
}

func (h *host) accountView() (bool, string) {
	h.loginMu.Lock()
	sess := h.session
	h.loginMu.Unlock()
	if sess.Access == "" {
		return false, ""
	}
	if time.Now().After(sess.Expires) {
		if !h.refreshSession(sess) {
			return false, ""
		}
		h.loginMu.Lock()
		sess = h.session
		h.loginMu.Unlock()
	}
	if sess.Access == "" {
		return false, ""
	}
	return true, sess.Account
}

func (h *host) refreshSession(sess accountSession) bool {
	access, refresh, err := refreshTokens(sess.Server, sess.Refresh)
	if err != nil {
		h.loginMu.Lock()
		h.session = accountSession{}
		h.loginMu.Unlock()
		_ = saveSession(accountSession{})
		return false
	}
	next := sess
	next.Access = access
	next.Refresh = refresh
	next.Expires = time.Now().Add(14 * time.Minute)
	h.loginMu.Lock()
	h.session = next
	h.loginMu.Unlock()
	_ = saveSession(next)
	return true
}

func (h *host) loadSession() {
	sess, err := readSession()
	if err != nil || sess.Access == "" {
		return
	}
	profiles := h.svc.GetServerProfiles()
	if len(profiles) == 0 || strings.TrimRight(profiles[0].URL, "/") != strings.TrimRight(sess.Server, "/") {
		return
	}
	h.loginMu.Lock()
	h.session = sess
	h.loginMu.Unlock()
}

func providerNames(server string) ([]string, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(server + "/.well-known/withsecrets")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery %d", resp.StatusCode)
	}
	var doc struct {
		AuthProviders []string `json:"authProviders"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return nil, err
	}
	return doc.AuthProviders, nil
}

func exchangeCode(server, code, verifier, state string) (string, string, error) {
	payload, _ := json.Marshal(map[string]string{
		"code": code, "codeVerifier": verifier, "state": state,
	})
	return tokenPair(server+"/api/v1/auth/desktop/exchange", payload, "")
}

func refreshTokens(server, refresh string) (string, string, error) {
	payload, _ := json.Marshal(map[string]string{"refreshToken": refresh})
	return tokenPair(server+"/api/v1/auth/refresh", payload, "")
}

func tokenPair(endpoint string, payload []byte, bearer string) (string, string, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("token %d", resp.StatusCode)
	}
	var doc struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return "", "", err
	}
	if doc.AccessToken == "" || doc.RefreshToken == "" {
		return "", "", fmt.Errorf("token")
	}
	return doc.AccessToken, doc.RefreshToken, nil
}

func fetchAccount(server, access string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, server+"/api/v1/accounts/me", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("account %d", resp.StatusCode)
	}
	var doc struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return "", err
	}
	if doc.ID == "" {
		return "", fmt.Errorf("account")
	}
	return doc.ID, nil
}

func sessionPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "withsecrets", "desktop-session.json"), nil
}

func readSession() (accountSession, error) {
	path, err := sessionPath()
	if err != nil {
		return accountSession{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return accountSession{}, err
	}
	var sess accountSession
	if err := json.Unmarshal(raw, &sess); err != nil {
		return accountSession{}, err
	}
	return sess, nil
}

func saveSession(sess accountSession) error {
	path, err := sessionPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if sess.Access == "" {
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	body, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

func randomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func s256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func openBrowser(raw string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", raw)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", raw)
	default:
		cmd = exec.Command("xdg-open", raw)
	}
	return cmd.Start()
}
