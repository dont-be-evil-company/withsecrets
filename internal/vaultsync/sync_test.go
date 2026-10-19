package vaultsync

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
)

func TestCacheStoresCiphertextOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.sqlite")
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := []byte("SENTINEL-PLAINTEXT-super-secret")
	key, err := vaultcrypto.NewMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	env, err := vaultcrypto.SealItem(key, vaultcrypto.AAD{
		AccountID: "acct", InstanceID: "inst", VaultID: "vault", ItemID: "item", Revision: 1,
	}, sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Put("inst", "acct", "vault", "item", 1, env); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, sentinel) {
		t.Fatal("plaintext sentinel stored in the offline cache")
	}
}

func TestConflictKeepsBoth(t *testing.T) {
	push, conflict := Decide("item", 1, 2, 2, []byte("local"), []byte("remote"))
	if push || conflict == nil {
		t.Fatal("diverged edits must surface a conflict")
	}
	if bytes.Equal(conflict.LocalCiphertext, conflict.RemoteCiphertext) {
		t.Fatal("conflict collapsed the two versions")
	}
	push, conflict = Decide("item", 1, 2, 1, []byte("local"), []byte("old"))
	if !push || conflict != nil {
		t.Fatal("remote still at base should push")
	}
}
