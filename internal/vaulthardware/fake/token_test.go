package fake

import (
	"bytes"
	"testing"

	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
)

func TestTwoTokensUnlockWithoutRecoverySecret(t *testing.T) {
	master, err := vaultcrypto.NewMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	primary := NewToken("primary-serial", "654321")
	spare := NewToken("spare-serial", "135790")
	credA, err := primary.MakeCredential("654321", true)
	if err != nil {
		t.Fatal(err)
	}
	credB, err := spare.MakeCredential("135790", true)
	if err != nil {
		t.Fatal(err)
	}
	saltA := bytes.Repeat([]byte{1}, 32)
	saltB := bytes.Repeat([]byte{2}, 32)
	ctx := func(id string) vaultcrypto.SlotContext {
		return vaultcrypto.SlotContext{AccountID: "a", InstanceID: "i", GenerationID: "g", SlotID: id}
	}
	slotA, err := vaultcrypto.EnrollFIDO2(master, ctx("a"), credA, saltA, func(id, salt []byte) ([]byte, error) {
		return primary.HMACSecret(id, salt, "654321", true)
	})
	if err != nil {
		t.Fatal(err)
	}
	slotB, err := vaultcrypto.EnrollFIDO2(master, ctx("b"), credB, saltB, func(id, salt []byte) ([]byte, error) {
		return spare.HMACSecret(id, salt, "135790", true)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := vaultcrypto.DistinctTokens(
		vaultcrypto.TokenProof{Serial: primary.Serial, CredentialID: credA},
		vaultcrypto.TokenProof{Serial: spare.Serial, CredentialID: credB},
	); err != nil {
		t.Fatal(err)
	}
	primary.Unplugged = true
	got, err := vaultcrypto.UnwrapFIDO2(slotB, func(id, salt []byte) ([]byte, error) {
		return spare.HMACSecret(id, salt, "135790", true)
	})
	if err != nil || !bytes.Equal(got, master) {
		t.Fatalf("spare key: %v", err)
	}
	if _, err := vaultcrypto.UnwrapFIDO2(slotA, func(id, salt []byte) ([]byte, error) {
		return primary.HMACSecret(id, salt, "654321", true)
	}); err == nil {
		t.Fatal("unplugged primary should fail")
	}
	_ = slotA
}

func TestPINTouchAndPIVGuards(t *testing.T) {
	tok := NewToken("s", "999999")
	if _, err := tok.HMACSecret([]byte("x"), []byte("y"), "999999", false); err == nil {
		t.Fatal("touch")
	}
	for i := 0; i < 8; i++ {
		_, _ = tok.MakeCredential("nope", true)
	}
	if _, err := tok.MakeCredential("999999", true); err == nil {
		t.Fatal("expected lockout")
	}
	piv := NewToken("p", "123456")
	if _, err := piv.GeneratePIV("123456", "always"); err == nil {
		t.Fatal("factory pin")
	}
	piv.PIN = "246810"
	if _, err := piv.GeneratePIV("246810", "never"); err == nil {
		t.Fatal("touch never")
	}
	if _, err := piv.GeneratePIV("246810", "always"); err != nil {
		t.Fatal(err)
	}
	if _, err := piv.GeneratePIV("246810", "always"); err == nil {
		t.Fatal("occupied")
	}
}
