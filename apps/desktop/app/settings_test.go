package app

import (
	"encoding/base32"
	"testing"
	"time"

	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
)

func TestUpdateDeleteAndIdle(t *testing.T) {
	svc := New()
	if _, err := svc.BootstrapPassphrase("correct horse battery"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PutDecryptedItem(ItemBody{Kind: "wifi", Title: "Cafe"}); err != nil {
		t.Fatal(err)
	}
	id := svc.Search("cafe")[0].ID
	if _, err := svc.PutDecryptedItem(ItemBody{
		ID: id, Kind: "wifi", Title: "Office",
		Fields: map[string]string{"ssid": "office", "password": "secret-net"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetItem(id)
	if err != nil || got.Title != "Office" || got.Fields["password"] != "secret-net" {
		t.Fatalf("update %#v %v", got, err)
	}
	if summaries := svc.Search(""); len(summaries) != 1 || summaries[0].Username != "" {
		t.Fatalf("list leaked or lost %#v", summaries)
	}
	raw := []byte("12345678901234567890")
	seed := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	if _, err := svc.PutDecryptedItem(ItemBody{ID: id, Kind: "login", Title: "Office", TOTP: seed}); err != nil {
		t.Fatal(err)
	}
	when := time.Unix(59, 0)
	code, remain, err := svc.CurrentTOTP(id, when)
	want, wantErr := vaultcrypto.TOTP(raw, when, 30, 6, "sha1")
	if err != nil || wantErr != nil || code != want || remain != 30-int(when.Unix()%30) {
		t.Fatalf("totp %q %d %v want %q", code, remain, err, want)
	}
	if err := svc.DeleteItem(id); err != nil {
		t.Fatal(err)
	}
	if len(svc.Search("")) != 0 {
		t.Fatal("deleted item still listed")
	}
	svc.mu.Lock()
	svc.lastActivity = time.Now().Add(-6 * time.Minute)
	svc.mu.Unlock()
	if !svc.IdleExpired(time.Now()) {
		t.Fatal("idle vault stayed unlocked")
	}
	if err := svc.ApplySettings(DesktopSettings{Theme: "light", IdleLockMinutes: 0, ClipboardClearSeconds: 30, CloseToTray: false}); err != nil {
		t.Fatal(err)
	}
	if svc.IdleExpired(time.Now().Add(time.Hour)) {
		t.Fatal("disabled idle lock still fired")
	}
}
