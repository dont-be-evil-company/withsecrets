package vaultsync

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/mistweaverco/withsecrets/apps/server/api"
	"github.com/mistweaverco/withsecrets/apps/server/storage"
	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
)

func TestReconcilePushPullConflict(t *testing.T) {
	ts := httptest.NewServer(testServer(t).Handler())
	defer ts.Close()
	access, _ := login(t, ts, "sync-user")
	client := NewClient(ts.URL, access)

	master, err := vaultcrypto.NewMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	defer vaultcrypto.Wipe(master)
	vaultKey, err := vaultcrypto.NewMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	defer vaultcrypto.Wipe(vaultKey)
	me, err := client.Me()
	if err != nil {
		t.Fatal(err)
	}
	vault, err := client.EnsurePersonal("")
	if err != nil {
		t.Fatal(err)
	}
	wrap, err := vaultcrypto.WrapVaultKeyForAccount(master, vaultKey, me.ID, me.Instance, vault.ID)
	if err != nil {
		t.Fatal(err)
	}
	itemID := "item000000000001"
	first, err := vaultcrypto.SealItem(vaultKey, vaultcrypto.AAD{
		AccountID: me.ID, InstanceID: me.Instance, VaultID: vault.ID, ItemID: itemID, Revision: 1,
	}, []byte(`{"title":"one"}`))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Reconcile(client, Snapshot{
		VaultID: vault.ID,
		Wrap:    wrap,
		Items: []LocalItem{{
			ID: itemID, Ciphertext: first, SealedRevision: 1, Dirty: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Pushed[itemID] != 1 {
		t.Fatalf("pushed %#v", result.Pushed)
	}

	opened, err := vaultcrypto.UnwrapVaultKeyForAccount(master, wrap, me.ID, me.Instance, vault.ID)
	if err != nil || !bytes.Equal(opened, vaultKey) {
		t.Fatalf("second device wrap: %v", err)
	}
	vaultcrypto.Wipe(opened)
	pulled, err := Reconcile(client, Snapshot{VaultID: vault.ID, Wrap: wrap})
	if err != nil {
		t.Fatal(err)
	}
	if len(pulled.Pulled) != 1 || !bytes.Equal(pulled.Pulled[0].Ciphertext, first) {
		t.Fatalf("pull %#v", pulled.Pulled)
	}
	plain, err := vaultcrypto.OpenItem(vaultKey, vaultcrypto.AAD{
		AccountID: me.ID, InstanceID: me.Instance, VaultID: vault.ID, ItemID: itemID, Revision: uint64(pulled.Pulled[0].Revision),
	}, pulled.Pulled[0].Ciphertext)
	if err != nil || string(plain) != `{"title":"one"}` {
		t.Fatalf("decrypt %v %s", err, plain)
	}

	edited, err := vaultcrypto.SealItem(vaultKey, vaultcrypto.AAD{
		AccountID: me.ID, InstanceID: me.Instance, VaultID: vault.ID, ItemID: itemID, Revision: 2,
	}, []byte(`{"title":"offline"}`))
	if err != nil {
		t.Fatal(err)
	}
	pushed, err := Reconcile(client, Snapshot{
		VaultID: vault.ID,
		Cursor:  result.Cursor,
		Wrap:    wrap,
		Items: []LocalItem{{
			ID: itemID, Ciphertext: edited, SealedRevision: 2, BaseRevision: 1, Dirty: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pushed.Pushed[itemID] != 2 {
		t.Fatalf("offline push %#v", pushed.Pushed)
	}
	got, err := client.GetItem(itemID)
	if err != nil || !bytes.Equal(got.Ciphertext, edited) {
		t.Fatalf("server item %v", err)
	}

	remote, err := vaultcrypto.SealItem(vaultKey, vaultcrypto.AAD{
		AccountID: me.ID, InstanceID: me.Instance, VaultID: vault.ID, ItemID: itemID, Revision: 3,
	}, []byte(`{"title":"remote"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.UpdateItem(itemID, remote, 2); err != nil {
		t.Fatal(err)
	}
	local, err := vaultcrypto.SealItem(vaultKey, vaultcrypto.AAD{
		AccountID: me.ID, InstanceID: me.Instance, VaultID: vault.ID, ItemID: itemID, Revision: 3,
	}, []byte(`{"title":"local"}`))
	if err != nil {
		t.Fatal(err)
	}
	conflict, err := Reconcile(client, Snapshot{
		VaultID: vault.ID,
		Cursor:  pushed.Cursor,
		Wrap:    wrap,
		Items: []LocalItem{{
			ID: itemID, Ciphertext: local, SealedRevision: 3, BaseRevision: 2, Dirty: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := conflict.Pushed[itemID]; ok {
		t.Fatal("conflict was pushed over the server")
	}
	if len(conflict.Conflicts) != 1 || !bytes.Equal(conflict.Conflicts[0].Ciphertext, remote) {
		t.Fatalf("conflict %#v", conflict.Conflicts)
	}
	if bytes.Equal(local, remote) {
		t.Fatal("fixture")
	}
}

func TestMiskeyedServerIDIsReplaced(t *testing.T) {
	ts := httptest.NewServer(testServer(t).Handler())
	defer ts.Close()
	access, _ := login(t, ts, "miskey-user")
	client := NewClient(ts.URL, access)
	me, err := client.Me()
	if err != nil {
		t.Fatal(err)
	}
	vault, err := client.EnsurePersonal("")
	if err != nil {
		t.Fatal(err)
	}
	key, err := vaultcrypto.NewMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	defer vaultcrypto.Wipe(key)
	itemID := "item000000000002"
	sealed, err := vaultcrypto.SealItem(key, vaultcrypto.AAD{
		AccountID: me.ID, InstanceID: me.Instance, VaultID: vault.ID, ItemID: itemID, Revision: 1,
	}, []byte(`{"title":"kept"}`))
	if err != nil {
		t.Fatal(err)
	}
	var minted Item
	if err := client.do(http.MethodPost, "/api/v1/vaults/"+vault.ID+"/items", map[string]any{
		"ciphertext": b64(sealed), "cryptoVersion": 1, "expectedRevision": 0,
	}, &minted); err != nil {
		t.Fatal(err)
	}
	if minted.ID == itemID || minted.ID == "" {
		t.Fatalf("server id %q", minted.ID)
	}
	result, err := Reconcile(client, Snapshot{
		VaultID: vault.ID,
		Items: []LocalItem{{
			ID: itemID, Ciphertext: sealed, SealedRevision: 1, BaseRevision: 1,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Pushed[itemID] != 1 || len(result.Pulled) != 0 {
		t.Fatalf("result pushed %#v pulled %#v", result.Pushed, result.Pulled)
	}
	got, err := client.GetItem(itemID)
	if err != nil || got.Deleted || !bytes.Equal(got.Ciphertext, sealed) {
		t.Fatalf("restored item %v %+v", err, got)
	}
	orphan, err := client.GetItem(minted.ID)
	if err != nil || !orphan.Deleted {
		t.Fatalf("orphan %v %+v", err, orphan)
	}
	plain, err := vaultcrypto.OpenItem(key, vaultcrypto.AAD{
		AccountID: me.ID, InstanceID: me.Instance, VaultID: vault.ID, ItemID: itemID, Revision: 1,
	}, got.Ciphertext)
	if err != nil || string(plain) != `{"title":"kept"}` {
		t.Fatalf("open %v %s", err, plain)
	}
}

func TestUnlockSlotUpload(t *testing.T) {
	ts := httptest.NewServer(testServer(t).Handler())
	defer ts.Close()
	access, _ := login(t, ts, "slot-user")
	client := NewClient(ts.URL, access)
	me, err := client.Me()
	if err != nil {
		t.Fatal(err)
	}
	master, err := vaultcrypto.NewMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	defer vaultcrypto.Wipe(master)
	slot, err := vaultcrypto.EnrollPassphrase(master, vaultcrypto.SlotContext{
		AccountID: me.ID, InstanceID: me.Instance, GenerationID: "g1", SlotID: "pass", Label: "Passphrase",
	}, "correct horse battery", vaultcrypto.TestArgon2(), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Enroll("passphrase", "g1"); err != nil {
		t.Fatal(err)
	}
	if err := client.PutSlot(RemoteSlot{
		ID: slot.Context.SlotID, GenerationID: slot.Context.GenerationID, Type: slot.Type,
		Label: slot.Label, AlgVersion: int(slot.Algorithm), PublicParams: slot.Params, Wrap: slot.Envelope,
	}); err != nil {
		t.Fatal(err)
	}
	slots, err := client.ListSlots()
	if err != nil || len(slots) != 1 {
		t.Fatalf("slots %v %d", err, len(slots))
	}
	got := vaultcrypto.UnlockSlot{
		Context: vaultcrypto.SlotContext{
			AccountID: me.ID, InstanceID: me.Instance, GenerationID: slots[0].GenerationID,
			SlotID: slots[0].ID, Label: slots[0].Label,
		},
		Type: slots[0].Type, Algorithm: uint8(slots[0].AlgVersion), Params: slots[0].PublicParams,
		Envelope: slots[0].Wrap, State: slots[0].State,
	}
	if _, err := vaultcrypto.UnwrapPassphrase(got, "correct horse battery"); err != nil {
		t.Fatal(err)
	}
}

func testServer(t *testing.T) *api.Server {
	t.Helper()
	srv, err := api.New(api.Config{PublicBaseURL: "http://127.0.0.1:8080", DevAuth: true}, storage.NewMemory(), nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetLog(io.Discard)
	return srv
}

func login(t *testing.T, ts *httptest.Server, subject string) (string, string) {
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
	httpClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := httpClient.Get(ts.URL + "/auth/desktop/start?" + q.Encode())
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
	code := loc.Query().Get("code")
	body, _ := json.Marshal(map[string]string{"code": code, "codeVerifier": verifier, "state": state})
	res, err = http.Post(ts.URL+"/api/v1/auth/desktop/exchange", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil || out.AccessToken == "" {
		t.Fatal(err)
	}
	return out.AccessToken, out.RefreshToken
}

func s256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
