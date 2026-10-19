package vaultcrypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"testing"
	"time"
)

func ctx(slot string) SlotContext {
	return SlotContext{
		AccountID: "acct-1", InstanceID: "inst-1", GenerationID: "gen-1", SlotID: slot, Label: slot,
	}
}

func mustMaster(t *testing.T) []byte {
	t.Helper()
	k, err := NewMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestPassphraseSlot(t *testing.T) {
	master := mustMaster(t)
	_, err := EnrollPassphrase(master, ctx("p"), "correct horse", TestArgon2(), false)
	if err == nil {
		t.Fatal("low cost must be refused without allowLowCost")
	}
	slot, err := EnrollPassphrase(master, ctx("p"), "correct horse", TestArgon2(), true)
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnwrapPassphrase(slot, "correct horse")
	if err != nil || !ctEqual(got, master) {
		t.Fatalf("unwrap: %v", err)
	}
	if _, err := UnwrapPassphrase(slot, "wrong"); err == nil {
		t.Fatal("wrong passphrase")
	}
	slot.Context.InstanceID = "other"
	if _, err := UnwrapPassphrase(slot, "correct horse"); err == nil {
		t.Fatal("cross-instance unwrap")
	}
}

func TestRecoveryAndCorrupt(t *testing.T) {
	master := mustMaster(t)
	secret, err := NewRecoverySecret()
	if err != nil {
		t.Fatal(err)
	}
	slot, err := EnrollRecovery(master, secret, ctx("r"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnwrapRecovery(slot, secret)
	if err != nil || !ctEqual(got, master) {
		t.Fatal(err)
	}
	bad := bytes.Clone(secret)
	bad[0] ^= 0xff
	if _, err := UnwrapRecovery(slot, bad); err == nil {
		t.Fatal("wrong recovery secret")
	}
	slot.Envelope[len(slot.Envelope)-1] ^= 0xff
	if _, err := UnwrapRecovery(slot, secret); err == nil {
		t.Fatal("corrupt envelope")
	}
}

func TestFIDO2TwoTokensAndSpareRecovery(t *testing.T) {
	master := mustMaster(t)
	credA := bytes.Repeat([]byte{0x11}, 16)
	credB := bytes.Repeat([]byte{0x22}, 16)
	saltA := bytes.Repeat([]byte{0xA1}, 32)
	saltB := bytes.Repeat([]byte{0xB2}, 32)
	secrets := map[string][]byte{
		string(credA): bytes.Repeat([]byte{0x01}, 32),
		string(credB): bytes.Repeat([]byte{0x02}, 32),
	}
	hmacFor := func(cred []byte) HMACSecretFunc {
		return func(got, salt []byte) ([]byte, error) {
			key, ok := secrets[string(got)]
			if !ok || !bytes.Equal(got, cred) {
				return nil, ErrToken
			}
			mac := sha256.Sum256(append(key, salt...))
			return mac[:], nil
		}
	}
	slotA, err := EnrollFIDO2(master, ctx("a"), credA, saltA, hmacFor(credA))
	if err != nil {
		t.Fatal(err)
	}
	slotB, err := EnrollFIDO2(master, ctx("b"), credB, saltB, hmacFor(credB))
	if err != nil {
		t.Fatal(err)
	}
	if err := DistinctTokens(
		TokenProof{Serial: "yk-1", CredentialID: credA},
		TokenProof{Serial: "yk-2", CredentialID: credB},
	); err != nil {
		t.Fatal(err)
	}
	if err := HardwareOnlyReady([]UnlockSlot{slotA, slotB}, nil); err != nil {
		t.Fatal(err)
	}
	// Clean device: only the spare token is present. No recovery secret.
	got, err := UnwrapFIDO2(slotB, hmacFor(credB))
	if err != nil || !ctEqual(got, master) {
		t.Fatalf("spare unlock: %v", err)
	}
	if _, err := UnwrapFIDO2(slotA, hmacFor(credB)); err == nil {
		t.Fatal("wrong token")
	}
	vaultKey, _ := NewMasterKey()
	wrap, err := WrapVaultKeyForAccount(master, vaultKey, "acct-1", "inst-1", "vault-1")
	if err != nil {
		t.Fatal(err)
	}
	openedVault, err := UnwrapVaultKeyForAccount(got, wrap, "acct-1", "inst-1", "vault-1")
	if err != nil || !ctEqual(openedVault, vaultKey) {
		t.Fatal(err)
	}
	if _, err := UnwrapVaultKeyForAccount(got, wrap, "acct-1", "inst-OTHER", "vault-1"); err == nil {
		t.Fatal("vault wrap replayed onto another instance")
	}
	sentinel := []byte("SENTINEL-PLAINTEXT-super-secret")
	itemEnv, err := SealItem(openedVault, AAD{
		AccountID: "acct-1", InstanceID: "inst-1", VaultID: "vault-1", ItemID: "item-1", Revision: 1,
	}, sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(itemEnv, sentinel) || bytes.Contains(wrap, sentinel) {
		t.Fatal("plaintext leaked into envelope")
	}
	pt, err := OpenItem(openedVault, AAD{
		AccountID: "acct-1", InstanceID: "inst-1", VaultID: "vault-1", ItemID: "item-1", Revision: 1,
	}, itemEnv)
	if err != nil || !bytes.Equal(pt, sentinel) {
		t.Fatal(err)
	}
	if _, err := OpenItem(openedVault, AAD{
		AccountID: "acct-1", InstanceID: "inst-1", VaultID: "vault-2", ItemID: "item-1", Revision: 1,
	}, itemEnv); err == nil {
		t.Fatal("cross-vault swap")
	}
	if _, err := OpenItem(openedVault, AAD{
		AccountID: "acct-1", InstanceID: "inst-1", VaultID: "vault-1", ItemID: "item-1", Revision: 2,
	}, itemEnv); err == nil {
		t.Fatal("revision swap")
	}
}

func TestPIVSlot(t *testing.T) {
	master := mustMaster(t)
	good, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EnrollPIV(master, ctx("piv"), &good.PublicKey, "never", "always"); err == nil {
		t.Fatal("touch never")
	}
	slot, err := EnrollPIV(master, ctx("piv"), &good.PublicKey, "always", "always")
	if err != nil {
		t.Fatal(err)
	}
	dec := func(priv *rsa.PrivateKey) OAEPDecrypter {
		return func(ct, label []byte) ([]byte, error) {
			return rsa.DecryptOAEP(sha256.New(), rand.Reader, priv, ct, label)
		}
	}
	got, err := UnwrapPIV(slot, dec(good))
	if err != nil || !ctEqual(got, master) {
		t.Fatalf("piv unwrap: %v", err)
	}
	if _, err := UnwrapPIV(slot, dec(other)); err == nil {
		t.Fatal("wrong piv key")
	}
}

func TestPolicyRemovalRotationAndManifest(t *testing.T) {
	hw := []UnlockSlot{
		{Context: SlotContext{SlotID: "a"}, Type: SlotFIDO2, State: StateActive},
		{Context: SlotContext{SlotID: "b"}, Type: SlotPIV, State: StateActive},
		{Context: SlotContext{SlotID: "c"}, Type: SlotFIDO2, State: StatePending},
	}
	if err := CanRemoveSlot(PolicyHardwareOnly, hw, "a"); err == nil {
		t.Fatal("removing one of two verified hardware slots must fail")
	}
	hw = append(hw, UnlockSlot{Context: SlotContext{SlotID: "d"}, Type: SlotFIDO2, State: StateActive})
	if err := CanRemoveSlot(PolicyHardwareOnly, hw, "a"); err != nil {
		t.Fatal(err)
	}
	if err := DistinctTokens(
		TokenProof{CredentialID: []byte("a")},
		TokenProof{CredentialID: []byte("b")},
	); err == nil {
		t.Fatal("missing reinsert proof")
	}
	if err := DistinctTokens(
		TokenProof{CredentialID: []byte("same"), Serial: "1"},
		TokenProof{CredentialID: []byte("same"), Serial: "2"},
	); err == nil {
		t.Fatal("same credential")
	}
	pending := Generation{
		ID: "g2", Sequence: 2, State: StatePending,
		Slots: []UnlockSlot{
			{Context: SlotContext{SlotID: "a"}, Type: SlotFIDO2, State: StateActive},
			{Context: SlotContext{SlotID: "b"}, Type: SlotPIV, State: StatePending},
		},
	}
	if err := CanCommitGeneration(pending, PolicyHardwareOnly); err == nil {
		t.Fatal("unverified slot must block commit")
	}
	pending.Slots[1].State = StateActive
	if err := CanCommitGeneration(pending, PolicyHardwareOnly); err != nil {
		t.Fatal(err)
	}
	keys, err := NewDeviceKeys()
	if err != nil {
		t.Fatal(err)
	}
	m := Manifest{
		AccountID: "acct-1", InstanceID: "inst-1", GenerationID: "g2",
		RecoveryPolicy: PolicyHardwareOnly, Sequence: 2,
		Entries: []ManifestEntry{{SlotID: "b", Type: SlotPIV, State: StateActive, Hash: HashEnvelope([]byte("env-b"))}},
	}
	sig, err := SignManifest(keys.Sign, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyManifest(keys.SignPub, sig, m); err != nil {
		t.Fatal(err)
	}
	m.Sequence = 1
	if err := VerifyManifest(keys.SignPub, sig, m); err == nil {
		t.Fatal("tampered manifest")
	}
	if err := CheckSequence(2, 2); err == nil {
		t.Fatal("rollback")
	}
	if err := CheckSequence(2, 3); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceEnvelopeAndChunks(t *testing.T) {
	vaultKey := mustMaster(t)
	dev, err := NewDeviceKeys()
	if err != nil {
		t.Fatal(err)
	}
	env, err := SealForDevice(vaultKey, "vault-1", "inst-1", 1, dev.Box.PublicKey(), dev.Sign)
	if err != nil {
		t.Fatal(err)
	}
	got, err := OpenForDevice(env, "inst-1", dev.Box)
	if err != nil || !ctEqual(got, vaultKey) {
		t.Fatal(err)
	}
	env.Envelope[len(env.Envelope)-2] ^= 0x5a
	if _, err := OpenForDevice(env, "inst-1", dev.Box); err == nil {
		t.Fatal("forged envelope")
	}
	aad := AAD{AccountID: "a", InstanceID: "i", VaultID: "v", ItemID: "file"}
	c0, err := SealChunk(vaultKey, aad, 0, []byte("chunk-zero-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenChunk(vaultKey, aad, 1, c0); err == nil {
		t.Fatal("chunk swap")
	}
	pt, err := OpenChunk(vaultKey, aad, 0, c0)
	if err != nil || string(pt) != "chunk-zero-bytes" {
		t.Fatal(err)
	}
}

func TestPasswordAndTOTP(t *testing.T) {
	pw, err := GeneratePassword(20, true, true, true, true)
	if err != nil || len(pw) != 20 {
		t.Fatal(err)
	}
	phrase, bits, err := GeneratePassphrase(6, DefaultWordlist)
	if err != nil || bits < 36 || phrase == "" {
		t.Fatalf("%s %d %v", phrase, bits, err)
	}
	// RFC 6238 appendix B, SHA1, 8 digits, secret "12345678901234567890", T=59.
	secret := []byte("12345678901234567890")
	code, err := TOTP(secret, time.Unix(59, 0), 30, 8, "sha1")
	if err != nil || code != "94287082" {
		t.Fatalf("totp: %s %v", code, err)
	}
}

func FuzzParseEnvelope(f *testing.F) {
	f.Add([]byte("WSEC"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseEnvelope(data)
	})
}
