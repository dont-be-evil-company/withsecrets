package api

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mistweaverco/withsecrets/apps/server/storage"
	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
)

func newTestServer(t *testing.T, dev bool) *httptest.Server {
	t.Helper()
	cfg := Config{PublicBaseURL: "http://127.0.0.1:8080", DevAuth: dev}
	srv, err := New(cfg, storage.NewMemory(), nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetLog(io.Discard)
	return httptest.NewServer(srv.Handler())
}

func TestConfigRejectsPublicHTTPAndDevAuth(t *testing.T) {
	if err := (Config{PublicBaseURL: "http://vault.example.com"}).Validate(); err == nil {
		t.Fatal("public http")
	}
	if err := (Config{PublicBaseURL: "https://vault.example.com", DevAuth: true}).Validate(); err == nil {
		t.Fatal("dev auth on public host")
	}
	if err := (Config{PublicBaseURL: "http://127.0.0.1:8080", DevAuth: true}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSetupFailsClosed(t *testing.T) {
	ts := newTestServer(t, false)
	defer ts.Close()
	res, err := http.Get(ts.URL + "/auth/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status %d", res.StatusCode)
	}
	res, err = http.Get(ts.URL + "/healthz")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("health %v %d", err, res.StatusCode)
	}
	_ = res.Body.Close()
}

func login(t *testing.T, ts *httptest.Server, subject string) (access, refresh string) {
	t.Helper()
	verifier := "verifier-secret-value-0123456789abcdef"
	state := "state-" + subject
	q := url.Values{}
	q.Set("provider", "dev")
	q.Set("state", state)
	q.Set("code_challenge", s256(verifier))
	q.Set("code_challenge_method", "S256")
	q.Set("callback", "http://127.0.0.1:9/auth/callback")
	q.Set("subject", subject)
	q.Set("email", subject+"@example.com")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Get(ts.URL + "/auth/desktop/start?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("start %d %s", res.StatusCode, b)
	}
	loc, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if loc.Query().Get("access_token") != "" || loc.Query().Get("refresh_token") != "" {
		t.Fatal("token leaked into redirect")
	}
	code := loc.Query().Get("code")
	body, _ := json.Marshal(map[string]string{"code": code, "codeVerifier": verifier, "state": state})
	res, err = http.Post(ts.URL+"/api/v1/auth/desktop/exchange", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("exchange %d %s", res.StatusCode, b)
	}
	var out struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	res, err = http.Post(ts.URL+"/api/v1/auth/desktop/exchange", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode == 200 {
		t.Fatal("reused grant")
	}
	return out.AccessToken, out.RefreshToken
}

func authed(t *testing.T, method, url, token string, body any) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestDesktopHandoffVaultAndIsolation(t *testing.T) {
	ts := newTestServer(t, true)
	defer ts.Close()
	access, refresh := login(t, ts, "alice")
	bad := url.Values{}
	bad.Set("state", "x")
	bad.Set("code_challenge", s256("verifier-secret-value-0123456789abcdef"))
	bad.Set("code_challenge_method", "S256")
	bad.Set("callback", "https://evil.example/steal")
	res, err := http.Get(ts.URL + "/auth/desktop/start?" + bad.Encode())
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("evil callback %d", res.StatusCode)
	}

	res = authed(t, http.MethodPost, ts.URL+"/api/v1/accounts/enroll", access, map[string]any{
		"recoveryPolicy": vaultcrypto.PolicyHardwareOnly, "sequence": 1,
	})
	if res.StatusCode != 200 {
		t.Fatalf("enroll %d", res.StatusCode)
	}
	_ = res.Body.Close()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/devices/enrollment-request", access, map[string]string{
		"signPub": base64.StdEncoding.EncodeToString(pub), "boxPub": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32)),
		"fingerprint": "fp-alice",
	})
	var dev struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(res.Body).Decode(&dev); err != nil || dev.Status != "active" {
		t.Fatalf("device %v %s", err, dev.Status)
	}
	_ = res.Body.Close()

	wrap := []byte("WSEC-not-a-real-key-but-opaque")
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/accounts/unlock-slots", access, map[string]any{
		"slotType": "fido2-hmac-secret", "publicLabel": "primary", "algVersion": 5,
		"encryptedKeyWrap": base64.StdEncoding.EncodeToString(wrap), "generationId": "g1",
	})
	var slot struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&slot)
	_ = res.Body.Close()
	if slot.ID == "" {
		t.Fatal("slot")
	}
	proof := ed25519.Sign(priv, slotProof(storage.Slot{ID: slot.ID, Type: "fido2-hmac-secret", Wrap: wrap}))
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/accounts/unlock-slots/"+slot.ID+"/verify", access, map[string]string{
		"deviceId": dev.ID, "clientProof": base64.StdEncoding.EncodeToString(proof),
	})
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("verify %d %s", res.StatusCode, b)
	}
	_ = res.Body.Close()

	res = authed(t, http.MethodPost, ts.URL+"/api/v1/vaults", access, map[string]string{"nameHint": "personal"})
	var vault storage.Vault
	if err := json.NewDecoder(res.Body).Decode(&vault); err != nil || vault.ID == "" {
		t.Fatal(err)
	}
	_ = res.Body.Close()

	sentinel := []byte("SENTINEL-PLAINTEXT-super-secret")
	ct := []byte("ciphertext-that-does-not-contain-the-sentinel")
	if bytes.Contains(ct, sentinel) {
		t.Fatal("fixture")
	}
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/vaults/"+vault.ID+"/items", access, map[string]any{
		"password": string(sentinel),
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatal("plaintext accepted")
	}
	_ = res.Body.Close()

	res = authed(t, http.MethodPost, ts.URL+"/api/v1/vaults/"+vault.ID+"/items", access, map[string]any{
		"ciphertext": base64.StdEncoding.EncodeToString(ct), "cryptoVersion": 1, "idempotencyKey": "once",
	})
	var item map[string]any
	if err := json.NewDecoder(res.Body).Decode(&item); err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	raw, _ := json.Marshal(item)
	if bytes.Contains(raw, sentinel) {
		t.Fatal("sentinel in response")
	}
	id := item["id"].(string)
	res = authed(t, http.MethodPut, ts.URL+"/api/v1/items/"+id, access, map[string]any{
		"ciphertext": base64.StdEncoding.EncodeToString(append(ct, '2')), "cryptoVersion": 1, "expectedRevision": 1,
	})
	_ = res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("update %d", res.StatusCode)
	}
	res = authed(t, http.MethodPut, ts.URL+"/api/v1/items/"+id, access, map[string]any{
		"ciphertext": base64.StdEncoding.EncodeToString(append(ct, '3')), "cryptoVersion": 1, "expectedRevision": 1,
	})
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("conflict %d", res.StatusCode)
	}
	_ = res.Body.Close()

	bob, _ := login(t, ts, "bob")
	res = authed(t, http.MethodGet, ts.URL+"/api/v1/vaults/"+vault.ID, bob, nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("idor %d", res.StatusCode)
	}
	_ = res.Body.Close()
	res = authed(t, http.MethodGet, ts.URL+"/api/v1/items/"+id, bob, nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("item idor %d", res.StatusCode)
	}
	_ = res.Body.Close()

	res = authed(t, http.MethodPost, ts.URL+"/api/v1/auth/refresh", "", map[string]string{"refreshToken": refresh})
	if res.StatusCode != 200 {
		t.Fatalf("refresh %d", res.StatusCode)
	}
	var rotated struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	_ = json.NewDecoder(res.Body).Decode(&rotated)
	_ = res.Body.Close()
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/auth/refresh", "", map[string]string{"refreshToken": refresh})
	if res.StatusCode == 200 {
		t.Fatal("refresh reuse")
	}
	_ = res.Body.Close()
	_ = rotated
}

func TestTeamRevokeRemovesFutureAccess(t *testing.T) {
	ts := newTestServer(t, true)
	defer ts.Close()
	alice, _ := login(t, ts, "owner")
	bob, _ := login(t, ts, "member")
	me := authed(t, http.MethodGet, ts.URL+"/api/v1/accounts/me", bob, nil)
	var who struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(me.Body).Decode(&who)
	_ = me.Body.Close()

	res := authed(t, http.MethodPost, ts.URL+"/api/v1/organizations", alice, map[string]string{"name": "Acme"})
	var org storage.Org
	_ = json.NewDecoder(res.Body).Decode(&org)
	_ = res.Body.Close()
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/organizations/"+org.ID+"/invitations", alice, map[string]string{"role": "member"})
	var inv struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(res.Body).Decode(&inv)
	_ = res.Body.Close()
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/invitations/"+inv.Token+"/accept", bob, map[string]string{})
	if res.StatusCode != 200 {
		t.Fatalf("accept %d", res.StatusCode)
	}
	_ = res.Body.Close()

	res = authed(t, http.MethodPost, ts.URL+"/api/v1/vaults", alice, map[string]string{"nameHint": "team", "orgId": org.ID})
	var vault storage.Vault
	_ = json.NewDecoder(res.Body).Decode(&vault)
	_ = res.Body.Close()
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/vaults/"+vault.ID+"/members", alice, map[string]string{
		"principalId": who.ID, "permission": "edit",
	})
	_ = res.Body.Close()
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/devices/enrollment-request", bob, map[string]string{
		"signPub": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)),
		"boxPub":  base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32)),
	})
	var dev struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&dev)
	_ = res.Body.Close()
	env := base64.StdEncoding.EncodeToString([]byte("encrypted-vault-key-for-bob"))
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/vaults/"+vault.ID+"/key-envelopes", alice, map[string]any{
		"recipientDeviceId": dev.ID, "ciphertext": env, "signature": base64.StdEncoding.EncodeToString([]byte("sig")), "generation": 1,
	})
	_ = res.Body.Close()

	ct := base64.StdEncoding.EncodeToString([]byte("team-ciphertext"))
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/vaults/"+vault.ID+"/items", alice, map[string]any{
		"ciphertext": ct, "cryptoVersion": 1,
	})
	var item struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&item)
	_ = res.Body.Close()
	res = authed(t, http.MethodGet, ts.URL+"/api/v1/items/"+item.ID, bob, nil)
	if res.StatusCode != 200 {
		t.Fatalf("member read %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if strings.Contains(string(body), "password") {
		t.Fatal("unexpected plaintext key")
	}

	res = authed(t, http.MethodDelete, ts.URL+"/api/v1/vaults/"+vault.ID+"/members/"+who.ID, alice, nil)
	_ = res.Body.Close()
	res = authed(t, http.MethodGet, ts.URL+"/api/v1/items/"+item.ID, bob, nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("revoked read %d", res.StatusCode)
	}
	_ = res.Body.Close()
	res = authed(t, http.MethodGet, ts.URL+"/api/v1/vaults/"+vault.ID+"/key-envelopes", bob, nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("revoked envelopes %d", res.StatusCode)
	}
	_ = res.Body.Close()

	// Organization role alone does not reveal a vault the admin was not granted.
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/vaults", alice, map[string]string{"nameHint": "private"})
	var private storage.Vault
	_ = json.NewDecoder(res.Body).Decode(&private)
	_ = res.Body.Close()
	res = authed(t, http.MethodGet, ts.URL+"/api/v1/vaults/"+private.ID, bob, nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("org member saw private vault %d", res.StatusCode)
	}
	_ = res.Body.Close()
}

func TestAttachmentChecksumAndSync(t *testing.T) {
	ts := newTestServer(t, true)
	defer ts.Close()
	access, _ := login(t, ts, "files")
	res := authed(t, http.MethodPost, ts.URL+"/api/v1/attachments/uploads", access, map[string]string{})
	var up struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&up)
	_ = res.Body.Close()
	chunk := []byte("encrypted-chunk-bytes")
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/attachments/uploads/"+up.ID+"/chunks/0", bytes.NewReader(chunk))
	req.Header.Set("Authorization", "Bearer "+access)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		SHA256 string `json:"sha256"`
	}
	_ = json.NewDecoder(res.Body).Decode(&saved)
	_ = res.Body.Close()
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/attachments/uploads/"+up.ID+"/complete", access, map[string]any{
		"chunks": []map[string]any{{"n": 0, "sha256": base64.StdEncoding.EncodeToString([]byte("nope"))}},
	})
	if res.StatusCode == 200 {
		t.Fatal("bad checksum accepted")
	}
	_ = res.Body.Close()
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/attachments/uploads/"+up.ID+"/complete", access, map[string]any{
		"chunks": []map[string]any{{"n": 0, "sha256": saved.SHA256}},
	})
	if res.StatusCode != 200 {
		t.Fatalf("complete %d", res.StatusCode)
	}
	_ = res.Body.Close()
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/attachments/"+up.ID+"/chunks/0", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if !bytes.Equal(got, chunk) {
		t.Fatalf("chunk mismatch %q", got)
	}

	res = authed(t, http.MethodPost, ts.URL+"/api/v1/vaults", access, map[string]string{"nameHint": "sync"})
	var vault storage.Vault
	_ = json.NewDecoder(res.Body).Decode(&vault)
	_ = res.Body.Close()
	res = authed(t, http.MethodGet, ts.URL+"/api/v1/sync/changes?cursor=0", access, nil)
	var page struct {
		Changes []storage.Change `json:"changes"`
	}
	_ = json.NewDecoder(res.Body).Decode(&page)
	_ = res.Body.Close()
	if len(page.Changes) == 0 {
		t.Fatal("expected a change")
	}
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/sync/events?cursor=0", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	stream, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if !bytes.Contains(stream, []byte("cursor")) {
		t.Fatalf("sse %s", stream)
	}
}

func TestWellKnown(t *testing.T) {
	ts := newTestServer(t, true)
	defer ts.Close()
	res, err := http.Get(ts.URL + "/.well-known/withsecrets")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var doc struct {
		InstanceID    string   `json:"instanceId"`
		AuthProviders []string `json:"authProviders"`
	}
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil || doc.InstanceID == "" {
		t.Fatal(err)
	}
	if len(doc.AuthProviders) != 1 || doc.AuthProviders[0] != "dev" {
		t.Fatalf("providers %#v", doc.AuthProviders)
	}
}

func TestPortalPagesAndCookie(t *testing.T) {
	portal := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("portal-page:" + r.URL.Path))
	})
	locked, err := New(Config{PublicBaseURL: "http://127.0.0.1:8080"}, storage.NewMemory(), portal)
	if err != nil {
		t.Fatal(err)
	}
	closed := httptest.NewServer(locked.Handler())
	defer closed.Close()
	home, err := http.Get(closed.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	homeBody, _ := io.ReadAll(home.Body)
	_ = home.Body.Close()
	if home.StatusCode != http.StatusServiceUnavailable || !bytes.Contains(homeBody, []byte("Sign-in is locked")) {
		t.Fatalf("setup %d %s", home.StatusCode, homeBody)
	}
	for _, path := range []string{"/server", "/account"} {
		res, err := http.Get(closed.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK || !bytes.Contains(body, []byte("portal-page:"+path)) {
			t.Fatalf("%s %d %s", path, res.StatusCode, body)
		}
	}

	open, err := New(Config{PublicBaseURL: "http://127.0.0.1:8080", DevAuth: true}, storage.NewMemory(), portal)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(open.Handler())
	defer ts.Close()
	verifier := "verifier-secret-value-0123456789abcdef"
	q := url.Values{}
	q.Set("provider", "dev")
	q.Set("state", "state-cookie")
	q.Set("code_challenge", s256(verifier))
	q.Set("code_challenge_method", "S256")
	q.Set("callback", "http://127.0.0.1:9/auth/callback")
	q.Set("subject", "ada")
	q.Set("email", "ada@example.com")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Get(ts.URL + "/auth/desktop/start?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "ws_access" {
			cookie = c
		}
	}
	if cookie == nil || !cookie.HttpOnly || cookie.Secure {
		t.Fatalf("cookie %#v", cookie)
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/accounts/me", nil)
	req.AddCookie(cookie)
	me, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer me.Body.Close()
	if me.StatusCode != http.StatusOK {
		t.Fatalf("me %d", me.StatusCode)
	}
}

func TestGitHubCallbackUsesItsOwnPKCE(t *testing.T) {
	desktopVerifier := "verifier-secret-value-0123456789abcdef"
	var challenge, gotVerifier string
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_ = r.ParseForm()
			gotVerifier = r.Form.Get("code_verifier")
			if gotVerifier == "" || gotVerifier == desktopVerifier || s256(gotVerifier) != challenge {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			if r.Form.Get("redirect_uri") != "http://127.0.0.1:8080/auth/callback/github" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"redirect_uri"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"gho_test"}`))
		case "/user":
			if r.Header.Get("Authorization") != "Bearer gho_test" || r.Header.Get("User-Agent") == "" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(`{"id":42,"name":"Ada"}`))
		case "/emails":
			_, _ = w.Write([]byte(`[{"email":"ada@example.com","primary":true,"verified":true}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer idp.Close()

	srv, err := New(Config{
		PublicBaseURL: "http://127.0.0.1:8080",
		GitHubID:      "gh-id",
		GitHubSecret:  "gh-secret",
	}, storage.NewMemory(), nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetProvider(oauthProvider{
		Name: "github", AuthURL: idp.URL + "/authorize",
		TokenURL: idp.URL + "/token", UserURL: idp.URL + "/user", EmailURL: idp.URL + "/emails",
		Issuer: "https://github.com", ClientID: "gh-id", ClientSecret: "gh-secret",
		Scopes: "read:user user:email",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	q := url.Values{}
	q.Set("provider", "github")
	q.Set("state", "desktop-state")
	q.Set("code_challenge", s256(desktopVerifier))
	q.Set("code_challenge_method", "S256")
	q.Set("callback", "http://127.0.0.1:9/auth/callback")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Get(ts.URL + "/auth/desktop/start?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("start %d %s", res.StatusCode, b)
	}
	loc, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	challenge = loc.Query().Get("code_challenge")
	if challenge == "" || challenge == s256(desktopVerifier) {
		t.Fatalf("provider challenge reused desktop challenge: %q", challenge)
	}
	callback := ts.URL + "/auth/callback/github?code=from-github&state=" + url.QueryEscape(loc.Query().Get("state")) + "&iss=" + url.QueryEscape("https://github.com/login/oauth")
	res, err = client.Get(callback)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("callback %d %s", res.StatusCode, b)
	}
	back, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if back.Query().Get("state") != "desktop-state" || back.Query().Get("code") == "" {
		t.Fatalf("handoff %s", back)
	}
	if s256(gotVerifier) != challenge {
		t.Fatalf("verifier did not match provider challenge")
	}
	body, _ := json.Marshal(map[string]string{
		"code": back.Query().Get("code"), "codeVerifier": desktopVerifier, "state": "desktop-state",
	})
	res, err = http.Post(ts.URL+"/api/v1/auth/desktop/exchange", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("exchange %d %s", res.StatusCode, b)
	}
}

func TestClientItemIDAndAccountWrap(t *testing.T) {
	ts := newTestServer(t, true)
	defer ts.Close()
	alice, _ := login(t, ts, "alice-sync")
	bob, _ := login(t, ts, "bob-sync")
	res := authed(t, http.MethodPost, ts.URL+"/api/v1/vaults", alice, map[string]string{"nameHint": "personal"})
	var vault storage.Vault
	if err := json.NewDecoder(res.Body).Decode(&vault); err != nil || vault.ID == "" {
		t.Fatal(err)
	}
	_ = res.Body.Close()

	itemID := "abc123item"
	ct := []byte("ciphertext-client-id")
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/vaults/"+vault.ID+"/items", alice, map[string]any{
		"id": itemID, "ciphertext": base64.StdEncoding.EncodeToString(ct), "cryptoVersion": 1,
	})
	var item map[string]any
	if err := json.NewDecoder(res.Body).Decode(&item); err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusCreated || item["id"] != itemID {
		t.Fatalf("create %d %#v", res.StatusCode, item)
	}
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/vaults/"+vault.ID+"/items", alice, map[string]any{
		"id": "short", "ciphertext": base64.StdEncoding.EncodeToString(ct), "cryptoVersion": 1,
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("short id %d", res.StatusCode)
	}
	_ = res.Body.Close()

	wrap := []byte("account-wrap-sentinel")
	res = authed(t, http.MethodPut, ts.URL+"/api/v1/vaults/"+vault.ID+"/account-key", alice, map[string]string{
		"accountKeyWrap": base64.StdEncoding.EncodeToString(wrap),
	})
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("wrap %d %s", res.StatusCode, b)
	}
	_ = res.Body.Close()
	res = authed(t, http.MethodGet, ts.URL+"/api/v1/vaults/"+vault.ID, alice, nil)
	var owner storage.Vault
	if err := json.NewDecoder(res.Body).Decode(&owner); err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if !bytes.Equal(owner.AccountKeyWrap, wrap) {
		t.Fatalf("owner wrap %q", owner.AccountKeyWrap)
	}

	me := authed(t, http.MethodGet, ts.URL+"/api/v1/accounts/me", bob, nil)
	var who struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(me.Body).Decode(&who); err != nil || who.ID == "" {
		t.Fatal(err)
	}
	_ = me.Body.Close()
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/vaults/"+vault.ID+"/members", alice, map[string]string{
		"principalId": who.ID, "permission": "view",
	})
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("share %d %s", res.StatusCode, b)
	}
	_ = res.Body.Close()
	res = authed(t, http.MethodGet, ts.URL+"/api/v1/vaults/"+vault.ID, bob, nil)
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || bytes.Contains(body, wrap) || bytes.Contains(body, []byte(base64.StdEncoding.EncodeToString(wrap))) {
		t.Fatalf("member saw wrap %d %s", res.StatusCode, body)
	}

	res = authed(t, http.MethodPost, ts.URL+"/api/v1/organizations", alice, map[string]string{"name": "Acme"})
	var org storage.Org
	if err := json.NewDecoder(res.Body).Decode(&org); err != nil || org.ID == "" {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/vaults", alice, map[string]string{"nameHint": "team", "orgId": org.ID})
	var team storage.Vault
	if err := json.NewDecoder(res.Body).Decode(&team); err != nil || team.ID == "" {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	res = authed(t, http.MethodPut, ts.URL+"/api/v1/vaults/"+team.ID+"/account-key", alice, map[string]string{
		"accountKeyWrap": base64.StdEncoding.EncodeToString(wrap),
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("org wrap %d", res.StatusCode)
	}
	_ = res.Body.Close()
}

func TestListOrgMembersShowsActiveDevices(t *testing.T) {
	ts := newTestServer(t, true)
	defer ts.Close()
	alice, _ := login(t, ts, "owner-list")
	bob, _ := login(t, ts, "member-list")
	carol, _ := login(t, ts, "stranger-list")
	me := authed(t, http.MethodGet, ts.URL+"/api/v1/accounts/me", bob, nil)
	var who struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(me.Body).Decode(&who); err != nil || who.ID == "" {
		t.Fatal(err)
	}
	_ = me.Body.Close()

	res := authed(t, http.MethodPost, ts.URL+"/api/v1/organizations", alice, map[string]string{"name": "Acme"})
	var org storage.Org
	if err := json.NewDecoder(res.Body).Decode(&org); err != nil || org.ID == "" {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/organizations/"+org.ID+"/invitations", alice, map[string]string{"role": "member"})
	var inv struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&inv); err != nil || inv.Token == "" {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/invitations/"+inv.Token+"/accept", bob, map[string]string{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("accept %d", res.StatusCode)
	}
	_ = res.Body.Close()
	box := bytes.Repeat([]byte{9}, 32)
	res = authed(t, http.MethodPost, ts.URL+"/api/v1/devices/enrollment-request", bob, map[string]string{
		"signPub": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, ed25519.PublicKeySize)),
		"boxPub":  base64.StdEncoding.EncodeToString(box),
	})
	var dev struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(res.Body).Decode(&dev); err != nil || dev.ID == "" || dev.Status != "active" {
		t.Fatalf("device %+v %v", dev, err)
	}
	_ = res.Body.Close()

	res = authed(t, http.MethodGet, ts.URL+"/api/v1/organizations/"+org.ID+"/members", alice, nil)
	var listed struct {
		Members []struct {
			ID      string `json:"id"`
			Role    string `json:"role"`
			Devices []struct {
				ID     string `json:"id"`
				BoxPub string `json:"boxPub"`
			} `json:"devices"`
		} `json:"members"`
	}
	if err := json.NewDecoder(res.Body).Decode(&listed); err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("members %d %v", res.StatusCode, err)
	}
	_ = res.Body.Close()
	var found bool
	for _, member := range listed.Members {
		if member.ID != who.ID {
			continue
		}
		found = member.Role == "member" && len(member.Devices) == 1 && member.Devices[0].ID == dev.ID && member.Devices[0].BoxPub == base64.StdEncoding.EncodeToString(box)
	}
	if !found {
		t.Fatalf("members %#v", listed.Members)
	}
	res = authed(t, http.MethodGet, ts.URL+"/api/v1/organizations/"+org.ID+"/members", carol, nil)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("stranger %d", res.StatusCode)
	}
	_ = res.Body.Close()
}
