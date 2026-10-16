package app

import (
	"testing"

	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
)

func TestLockDropsSecrets(t *testing.T) {
	svc := New()
	slot, err := svc.BootstrapPassphrase("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PutDecryptedItem(ItemBody{
		Kind: "login", Title: "Example", Username: "ada", Password: "SENTINEL-PLAINTEXT-super-secret", VaultID: "personal",
	}); err != nil {
		t.Fatal(err)
	}
	got := svc.Search("example")
	if len(got) != 1 || got[0].Title != "Example" {
		t.Fatalf("search %#v", got)
	}
	secret, err := svc.RevealField(got[0].ID, "password")
	if err != nil || secret == "" {
		t.Fatal(err)
	}
	svc.Lock()
	if svc.Unlocked() {
		t.Fatal("still unlocked")
	}
	if _, err := svc.RevealField(got[0].ID, "password"); err == nil {
		t.Fatal("reveal after lock")
	}
	if err := svc.UnlockWithPassphrase("nope", slot); err == nil {
		t.Fatal("wrong passphrase")
	}
	if err := svc.UnlockWithPassphrase("correct horse battery", slot); err != nil {
		t.Fatal(err)
	}
	again, err := svc.RevealField(got[0].ID, "password")
	if err != nil || again != secret {
		t.Fatalf("restore after unlock: %v %q", err, again)
	}
}

func TestBeginSetupStaysOnDevice(t *testing.T) {
	svc := New()
	if _, err := svc.BeginSetup("hardware", ""); err == nil {
		t.Fatal("hardware-only was stored before two tokens verified")
	}
	if svc.Unlocked() {
		t.Fatal("hardware refusal unlocked the vault")
	}
	res, err := svc.BeginSetup("recommended", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.RecoveryKey == "" || res.Policy != "hardware_plus_recovery" {
		t.Fatalf("setup %#v", res.Policy)
	}
	if _, err := svc.PutDecryptedItem(ItemBody{Kind: "login", Title: "Mail", Username: "ada", Password: "SENTINEL-PLAINTEXT"}); err != nil {
		t.Fatal(err)
	}
	svc.Lock()
	if err := svc.UnlockWithRecovery(res.RecoveryKey, res.RecSlot); err != nil {
		t.Fatal(err)
	}
	got, err := svc.RevealField(svc.Search("mail")[0].ID, "password")
	if err != nil || got != "SENTINEL-PLAINTEXT" {
		t.Fatalf("restore: %v %q", err, got)
	}
	if _, err := svc.BeginSetup("recommended", ""); err == nil {
		t.Fatal("second setup minted another recovery key")
	}
}

func TestFIDOSlotRoundTrip(t *testing.T) {
	svc := New()
	cred := []byte("credential-a")
	fn := func(id, salt []byte) ([]byte, error) {
		if len(id) == 0 || len(salt) != 32 {
			t.Fatalf("hmac args %d %d", len(id), len(salt))
		}
		out := make([]byte, 32)
		for i := range out {
			out[i] = salt[i] ^ byte(i+1)
		}
		return out, nil
	}
	slot, err := svc.EnrollFIDOSlot(cred, fn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PutDecryptedItem(ItemBody{Kind: "login", Title: "Mail", Username: "ada", Password: "SENTINEL-PLAINTEXT"}); err != nil {
		t.Fatal(err)
	}
	svc.Lock()
	if err := svc.UnlockWithFIDO(slot, fn); err != nil {
		t.Fatal(err)
	}
	got, err := svc.RevealField(svc.Search("mail")[0].ID, "password")
	if err != nil || got != "SENTINEL-PLAINTEXT" {
		t.Fatalf("restore %v %q", err, got)
	}
}

func TestBindIdentityResealsWithServerIDs(t *testing.T) {
	svc := New()
	if _, err := svc.BootstrapPassphrase("correct horse battery"); err != nil {
		t.Fatal(err)
	}
	rev, err := svc.PutDecryptedItem(ItemBody{Kind: "login", Title: "Example", Username: "ada", Password: "secret"})
	if err != nil || rev != 1 {
		t.Fatalf("create rev %d %v", rev, err)
	}
	id := svc.Search("example")[0].ID
	upload, err := svc.BindIdentity("user-1", "inst-1", "vaultabc123", nil)
	if err != nil || !upload {
		t.Fatalf("bind %v upload %v", err, upload)
	}
	env := svc.envelopes[id]
	if _, err := vaultcrypto.OpenItem(svc.vaultKey, vaultcrypto.AAD{
		AccountID: "local", InstanceID: "local-dev", VaultID: "personal", ItemID: id, Revision: 1,
	}, env); err == nil {
		t.Fatal("old associated data still opens the item")
	}
	plain, err := vaultcrypto.OpenItem(svc.vaultKey, vaultcrypto.AAD{
		AccountID: "user-1", InstanceID: "inst-1", VaultID: "vaultabc123", ItemID: id, Revision: 1,
	}, env)
	if err != nil {
		t.Fatal(err)
	}
	vaultcrypto.Wipe(plain)
	if svc.meta[id].sealed != 1 || !svc.meta[id].dirty || svc.meta[id].base != 0 {
		t.Fatalf("meta %+v", svc.meta[id])
	}
	if _, err := svc.PutDecryptedItem(ItemBody{ID: id, Kind: "login", Title: "Example", Username: "ada", Password: "next"}); err != nil {
		t.Fatal(err)
	}
	if svc.meta[id].sealed != 1 {
		t.Fatalf("unpushed edit sealed at %d", svc.meta[id].sealed)
	}
	svc.meta[id] = itemMeta{sealed: 1, base: 1}
	rev, err = svc.PutDecryptedItem(ItemBody{ID: id, Kind: "login", Title: "Example", Username: "ada", Password: "later"})
	if err != nil || rev != 2 {
		t.Fatalf("next rev %d %v", rev, err)
	}
	if _, err := vaultcrypto.OpenItem(svc.vaultKey, vaultcrypto.AAD{
		AccountID: "user-1", InstanceID: "inst-1", VaultID: "vaultabc123", ItemID: id, Revision: 2,
	}, svc.envelopes[id]); err != nil {
		t.Fatal(err)
	}
}

func TestProfilesStaySeparate(t *testing.T) {
	svc := New()
	if err := svc.AddProfile("home", "https://vault.example.com"); err != nil {
		t.Fatal(err)
	}
	profiles := svc.GetServerProfiles()
	if len(profiles) != 2 || profiles[0].URL == profiles[1].URL {
		t.Fatalf("profiles %#v", profiles)
	}
}
