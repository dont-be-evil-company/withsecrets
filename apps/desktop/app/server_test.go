package app

import "testing"

func TestNormalizeServerURL(t *testing.T) {
	ok, err := NormalizeServerURL(" https://vault.example.com/base ")
	if err != nil || ok != "https://vault.example.com/base" {
		t.Fatalf("%q %v", ok, err)
	}
	if _, err := NormalizeServerURL("http://vault.example.com"); err == nil {
		t.Fatal("public http accepted")
	}
	local, err := NormalizeServerURL("http://127.0.0.1:8080")
	if err != nil || local != "http://127.0.0.1:8080" {
		t.Fatalf("%q %v", local, err)
	}
	if _, err := NormalizeServerURL("https://user:secret@vault.example.com"); err == nil {
		t.Fatal("userinfo accepted")
	}
}

func TestSetDefaultServerLocksOnHostChange(t *testing.T) {
	svc := New()
	slot, err := svc.BootstrapPassphrase("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PutDecryptedItem(ItemBody{Kind: "login", Title: "Mail", Password: "SENTINEL-PLAINTEXT"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDefaultServer("https://vault.example.com"); err != nil {
		t.Fatal(err)
	}
	if svc.Unlocked() {
		t.Fatal("stayed unlocked after server change")
	}
	profiles := svc.GetServerProfiles()
	if profiles[0].URL != "https://vault.example.com" {
		t.Fatalf("profile %#v", profiles)
	}
	if err := svc.UnlockWithPassphrase("correct horse battery", slot); err != nil {
		t.Fatal(err)
	}
	got, err := svc.RevealField(svc.Search("mail")[0].ID, "password")
	if err != nil || got != "SENTINEL-PLAINTEXT" {
		t.Fatalf("item stayed on device: %v %q", err, got)
	}
}
