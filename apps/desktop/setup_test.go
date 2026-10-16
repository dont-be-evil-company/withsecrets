package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistweaverco/withsecrets/apps/desktop/app"
	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
	"github.com/mistweaverco/withsecrets/internal/vaulthardware/fido2"
)

func TestSetupCopyStaysInThisWindow(t *testing.T) {
	h := newHost()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:9"
	rec := httptest.NewRecorder()
	h.routes().ServeHTTP(rec, req)
	body := rec.Body.String()
	if strings.Contains(body, "Continue on the desktop") {
		t.Fatal("setup still tells the desktop user to open the desktop")
	}
	if !strings.Contains(body, "Finish both parts in this window") {
		t.Fatal("missing in-window recovery step")
	}
}

func TestSecondKeyMustDiffer(t *testing.T) {
	h := newHost()
	h.fido = []fidoRecord{{path: "/dev/hidraw0", serial: "111"}}
	plugged := []fido2.Info{{Path: "/dev/hidraw0", Serial: "111", HMAC: true}}
	if err := h.prepareFIDO("/dev/hidraw0", "111", plugged); !errors.Is(err, fido2.ErrSameToken) {
		t.Fatal(err)
	}
	if err := h.prepareFIDO("/dev/hidraw1", "222", append(plugged, fido2.Info{Path: "/dev/hidraw1", Serial: "222", HMAC: true})); err != nil {
		t.Fatal(err)
	}
}

func TestSetServerFromUI(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	h := newHost()
	payload, _ := json.Marshal(map[string]string{"url": "http://vault.example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/local/server", bytes.NewReader(payload))
	req.RemoteAddr = "127.0.0.1:9"
	rec := httptest.NewRecorder()
	h.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("public http %d %s", rec.Code, rec.Body.String())
	}
	payload, _ = json.Marshal(map[string]string{"url": "https://vault.example.com"})
	req = httptest.NewRequest(http.MethodPost, "/api/local/server", bytes.NewReader(payload))
	req.RemoteAddr = "127.0.0.1:9"
	rec = httptest.NewRecorder()
	h.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/local/status", nil)
	req.RemoteAddr = "127.0.0.1:9"
	rec = httptest.NewRecorder()
	h.routes().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "https://vault.example.com") {
		t.Fatalf("status %s", rec.Body.String())
	}
	h2 := newHost()
	if h2.svc.GetServerProfiles()[0].URL != "https://vault.example.com" {
		t.Fatalf("not restored %#v", h2.svc.GetServerProfiles())
	}
}

func TestPassphraseSetupExplainsTheMinimum(t *testing.T) {
	h := newHost()
	payload, _ := json.Marshal(map[string]string{"policy": "passphrase", "passphrase": "short"})
	req := httptest.NewRequest(http.MethodPost, "/api/local/setup", bytes.NewReader(payload))
	req.RemoteAddr = "127.0.0.1:9"
	rec := httptest.NewRecorder()
	h.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "at least 8 characters") {
		t.Fatalf("setup %d %s", rec.Code, rec.Body.String())
	}
}

func TestFinishSignInStoresAccount(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	vault := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/desktop/exchange":
			_ = json.NewEncoder(w).Encode(map[string]string{"accessToken": "access-1", "refreshToken": "refresh-1"})
		case "/api/v1/accounts/me":
			if r.Header.Get("Authorization") != "Bearer access-1" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "user-1", "instanceId": "inst"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer vault.Close()
	h := newHost()
	h.pending = pendingLogin{State: "state-1", Verifier: "verifier-secret-value-0123456789abcdef", Server: vault.URL}
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=once&state=nope", nil)
	req.RemoteAddr = "127.0.0.1:9"
	rec := httptest.NewRecorder()
	h.finishSignIn(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong state %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/auth/callback?code=once&state=state-1", nil)
	req.RemoteAddr = "127.0.0.1:9"
	rec = httptest.NewRecorder()
	h.finishSignIn(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "does not unlock") {
		t.Fatalf("callback %d %s", rec.Code, rec.Body.String())
	}
	signedIn, account := h.accountView()
	if !signedIn || account != "user-1" {
		t.Fatalf("session %v %q", signedIn, account)
	}
}

func TestVaultSurvivesRestart(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	h := newHost()
	payload, _ := json.Marshal(map[string]string{"policy": "recommended"})
	req := httptest.NewRequest(http.MethodPost, "/api/local/setup", bytes.NewReader(payload))
	req.RemoteAddr = "127.0.0.1:9"
	rec := httptest.NewRecorder()
	h.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup %d %s", rec.Code, rec.Body.String())
	}
	var setup map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &setup); err != nil {
		t.Fatal(err)
	}
	key, _ := setup["recoveryKey"].(string)
	item, _ := json.Marshal(map[string]string{"kind": "login", "title": "Mail", "password": "SENTINEL-PLAINTEXT"})
	req = httptest.NewRequest(http.MethodPost, "/api/local/items", bytes.NewReader(item))
	req.RemoteAddr = "127.0.0.1:9"
	rec = httptest.NewRecorder()
	h.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("item %d %s", rec.Code, rec.Body.String())
	}
	home := os.Getenv("XDG_CONFIG_HOME")
	body, err := os.ReadFile(filepath.Join(home, "withsecrets", "desktop-vault.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "SENTINEL-PLAINTEXT") {
		t.Fatal("vault file contains item plaintext")
	}
	h2 := newHost()
	req = httptest.NewRequest(http.MethodGet, "/api/local/status", nil)
	req.RemoteAddr = "127.0.0.1:9"
	rec = httptest.NewRecorder()
	h2.routes().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `"bootstrapped":true`) || strings.Contains(rec.Body.String(), `"unlocked":true`) {
		t.Fatalf("restart status %s", rec.Body.String())
	}
	unlockBody, _ := json.Marshal(map[string]string{"recoveryKey": key})
	req = httptest.NewRequest(http.MethodPost, "/api/local/unlock", bytes.NewReader(unlockBody))
	req.RemoteAddr = "127.0.0.1:9"
	rec = httptest.NewRecorder()
	h2.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unlock %d %s", rec.Code, rec.Body.String())
	}
	got := h2.svc.Search("mail")
	if len(got) != 1 {
		t.Fatal(got)
	}
	secret, err := h2.svc.RevealField(got[0].ID, "password")
	if err != nil || secret != "SENTINEL-PLAINTEXT" {
		t.Fatalf("item %v %q", err, secret)
	}
}

func TestEnrolledKeySurvivesRestart(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	h := newHost()
	cred := []byte("credential-a")
	fn := func(id, salt []byte) ([]byte, error) {
		out := make([]byte, 32)
		for i := range out {
			out[i] = salt[i] ^ byte(i+1)
		}
		return out, nil
	}
	slot, err := h.svc.EnrollFIDOSlot(cred, fn)
	if err != nil {
		t.Fatal(err)
	}
	h.fido = []fidoRecord{{
		path: "/dev/hidraw0", serial: "111", cred: cred, slot: slot,
		proof: vaultcrypto.TokenProof{Serial: "111", CredentialID: append([]byte(nil), cred...)},
	}}
	if _, err := h.svc.PutDecryptedItem(app.ItemBody{Kind: "login", Title: "Mail", Password: "SENTINEL-PLAINTEXT"}); err != nil {
		t.Fatal(err)
	}
	if err := h.persist(); err != nil {
		t.Fatal(err)
	}
	h2 := newHost()
	if len(h2.fido) != 1 {
		t.Fatalf("keys %d", len(h2.fido))
	}
	if h2.svc.Unlocked() {
		t.Fatal("restart left the vault unlocked")
	}
	if err := h2.svc.UnlockWithFIDO(h2.fido[0].slot, fn); err != nil {
		t.Fatal(err)
	}
	secret, err := h2.svc.RevealField(h2.svc.Search("mail")[0].ID, "password")
	if err != nil || secret != "SENTINEL-PLAINTEXT" {
		t.Fatalf("item %v %q", err, secret)
	}
}

func TestSetupCreatesRecoveryKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	h := newHost()
	payload, _ := json.Marshal(map[string]string{"policy": "recommended"})
	req := httptest.NewRequest(http.MethodPost, "/api/local/setup", bytes.NewReader(payload))
	req.RemoteAddr = "127.0.0.1:9"
	rec := httptest.NewRecorder()
	h.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	key, _ := got["recoveryKey"].(string)
	if key == "" || got["policy"] != "hardware_plus_recovery" {
		t.Fatalf("body %#v", got)
	}
	h.svc.Lock()
	unlockBody, _ := json.Marshal(map[string]string{"recoveryKey": key})
	req = httptest.NewRequest(http.MethodPost, "/api/local/unlock", bytes.NewReader(unlockBody))
	req.RemoteAddr = "127.0.0.1:9"
	rec = httptest.NewRecorder()
	h.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unlock %d %s", rec.Code, rec.Body.String())
	}
}

func TestSettingsAndItemEdit(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	h := newHost()
	payload, _ := json.Marshal(map[string]any{
		"passphrase": "correct horse battery",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/local/bootstrap", bytes.NewReader(payload))
	req.RemoteAddr = "127.0.0.1:9"
	rec := httptest.NewRecorder()
	h.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bootstrap %d %s", rec.Code, rec.Body.String())
	}
	item, _ := json.Marshal(map[string]any{"kind": "note", "title": "Diary", "notes": "secret note"})
	req = httptest.NewRequest(http.MethodPost, "/api/local/items", bytes.NewReader(item))
	req.RemoteAddr = "127.0.0.1:9"
	rec = httptest.NewRecorder()
	h.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "secret note") {
		t.Fatalf("create leaked or failed %d %s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil || len(listed.Items) != 1 {
		t.Fatal(err, rec.Body.String())
	}
	updated, _ := json.Marshal(map[string]any{"kind": "note", "title": "Diary", "notes": "changed"})
	req = httptest.NewRequest(http.MethodPut, "/api/local/items/"+listed.Items[0].ID, bytes.NewReader(updated))
	req.RemoteAddr = "127.0.0.1:9"
	rec = httptest.NewRecorder()
	h.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/local/items/"+listed.Items[0].ID, nil)
	req.RemoteAddr = "127.0.0.1:9"
	rec = httptest.NewRecorder()
	h.routes().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "changed") {
		t.Fatalf("detail %s", rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodDelete, "/api/local/items/"+listed.Items[0].ID, nil)
	req.RemoteAddr = "127.0.0.1:9"
	rec = httptest.NewRecorder()
	h.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), listed.Items[0].ID) {
		t.Fatalf("delete %d %s", rec.Code, rec.Body.String())
	}
	settings, _ := json.Marshal(map[string]any{
		"theme": "light", "idleLockMinutes": 0, "clipboardClearSeconds": 15, "closeToTray": false,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/local/settings", bytes.NewReader(settings))
	req.RemoteAddr = "127.0.0.1:9"
	rec = httptest.NewRecorder()
	h.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("settings %d %s", rec.Code, rec.Body.String())
	}
	h2 := newHost()
	got := h2.svc.Settings()
	if got.Theme != "light" || got.IdleLockMinutes != 0 || got.ClipboardClearSeconds != 15 || got.CloseToTray {
		t.Fatalf("settings not restored %#v", got)
	}
}
