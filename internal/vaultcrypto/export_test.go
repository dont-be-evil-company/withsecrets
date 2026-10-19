package vaultcrypto

import "testing"

func TestExportRejectsPlaintextFields(t *testing.T) {
	key := mustMaster(t)
	env, err := SealItem(key, AAD{AccountID: "a", InstanceID: "i", VaultID: "v", ItemID: "item", Revision: 1}, []byte("SENTINEL-PLAINTEXT-super-secret"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeExport([][]byte{env})
	if err != nil {
		t.Fatal(err)
	}
	if bytesContains(raw, []byte("SENTINEL-PLAINTEXT-super-secret")) {
		t.Fatal("export leaked plaintext")
	}
	got, err := DecodeExport(raw)
	if err != nil || len(got) != 1 {
		t.Fatal(err)
	}
	if _, err := DecodeExport([]byte(`{"format":"withsecrets-vault-export","version":1,"password":"hunter2","items":[]}`)); err == nil {
		t.Fatal("plaintext password field accepted")
	}
}

func bytesContains(hay, needle []byte) bool {
	return len(needle) == 0 || (len(hay) >= len(needle) && indexOf(hay, needle) >= 0)
}

func indexOf(hay, needle []byte) int {
	for i := 0; i+len(needle) <= len(hay); i++ {
		ok := true
		for j := range needle {
			if hay[i+j] != needle[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}
